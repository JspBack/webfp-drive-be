package handlers

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"net/mail"
	"strings"
	"time"

	httphelper "github.com/JspBack/webfp-drive/helper/http"
	mailhelper "github.com/JspBack/webfp-drive/helper/mail"
	"github.com/JspBack/webfp-drive/store"
)

const (
	verificationCodeLength  = 6
	verificationCodeTTL     = 15 * time.Minute
	verificationResendAfter = time.Minute
)

type verifyEmailRequest struct {
	Email string `json:"email"`
	Code  string `json:"code"`
}

type resendVerificationRequest struct {
	Email string `json:"email"`
}

type verificationResponse struct {
	Message          string `json:"message"`
	Email            string `json:"email"`
	ExpiresInMinutes int    `json:"expires_in_minutes"`
}

type verificationRequiredResponse struct {
	Error string `json:"error"`
	Code  string `json:"code"`
}

// newVerificationCode returns a cryptographically random six-digit string.
func newVerificationCode() (string, error) {
	n, err := rand.Int(rand.Reader, big.NewInt(1_000_000))
	if err != nil {
		return "", fmt.Errorf("generate verification code: %w", err)
	}
	return fmt.Sprintf("%06d", n.Int64()), nil
}

// VerifyEmail godoc
// @Summary      Verify an email address
// @Description  Confirms the emailed code for a password account and returns session tokens, signing the user in.
// @Tags         auth
// @Accept       json
// @Produce      json
// @Param        request  body      verifyEmailRequest  true  "Email and code"
// @Success      200      {object}  authResponse
// @Failure      400      {object}  map[string]string
// @Failure      404      {object}  map[string]string
// @Failure      429      {object}  map[string]string
// @Failure      500      {object}  map[string]string
// @Router       /api/v1/auth/verify-email [post].
func (h *Handler) VerifyEmail(w http.ResponseWriter, r *http.Request) {
	var req verifyEmailRequest
	if err := h.decodeJSON(w, r, &req); err != nil {
		httphelper.Error(w, http.StatusBadRequest, err.Error())
		return
	}
	email, ok := parseVerificationEmail(w, req.Email)
	if !ok {
		return
	}
	code := strings.TrimSpace(req.Code)
	if len(code) != verificationCodeLength || strings.IndexFunc(code, func(r rune) bool { return r < '0' || r > '9' }) != -1 {
		httphelper.Error(w, http.StatusBadRequest, "six-digit verification code is required")
		return
	}
	if err := h.store.Users.ConsumeVerificationCode(r.Context(), email, code); err != nil {
		switch {
		case errors.Is(err, store.ErrVerificationInvalid):
			httphelper.Error(w, http.StatusBadRequest, "invalid verification code")
		case errors.Is(err, store.ErrVerificationExpired):
			httphelper.Error(w, http.StatusBadRequest, "verification code has expired, request a new one")
		case errors.Is(err, store.ErrVerificationExhausted):
			httphelper.Error(w, http.StatusTooManyRequests, "too many attempts, request a new code")
		default:
			h.internalError(w, r, "consume verification code", err)
		}
		return
	}
	user, err := h.store.Users.MarkVerified(r.Context(), email)
	if err != nil {
		if errors.Is(err, store.ErrUserNotFound) {
			httphelper.Error(w, http.StatusNotFound, "account not found")
			return
		}
		h.internalError(w, r, "mark email verified", err)
		return
	}
	tokens, err := h.store.Users.CreateSession(r.Context(), user.ID)
	if err != nil {
		h.internalError(w, r, "create verified session", err)
		return
	}
	httphelper.JSON(w, http.StatusOK, authResponse{
		User: user, Token: tokens.AccessToken, RefreshToken: tokens.RefreshToken,
	})
}

// ResendVerification godoc
// @Summary      Resend the verification code
// @Description  Issues a fresh code for an unverified password account. Unknown or verified addresses answer identically to avoid probing.
// @Tags         auth
// @Accept       json
// @Produce      json
// @Param        request  body      resendVerificationRequest  true  "Email address"
// @Success      202      {object}  map[string]string
// @Failure      400      {object}  map[string]string
// @Failure      429      {object}  map[string]string
// @Failure      500      {object}  map[string]string
// @Router       /api/v1/auth/resend-verification [post].
func (h *Handler) ResendVerification(w http.ResponseWriter, r *http.Request) {
	var req resendVerificationRequest
	if err := h.decodeJSON(w, r, &req); err != nil {
		httphelper.Error(w, http.StatusBadRequest, err.Error())
		return
	}
	email, ok := parseVerificationEmail(w, req.Email)
	if !ok {
		return
	}
	allowed, err := h.store.Users.TryVerificationCooldown(r.Context(), email, verificationResendAfter)
	if err != nil {
		h.internalError(w, r, "check verification cooldown", err)
		return
	}
	if !allowed {
		httphelper.Error(w, http.StatusTooManyRequests, "please wait a minute before requesting another code")
		return
	}
	user, err := h.store.Users.GetByEmail(r.Context(), email)
	switch {
	case errors.Is(err, store.ErrUserNotFound):
		// Same success shape as a real send; see the summary.
	case err != nil:
		h.internalError(w, r, "look up user for verification resend", err)
		return
	case user.EmailVerified:
		// Nothing to send; same success shape as a real send.
	default:
		code, genErr := newVerificationCode()
		if genErr != nil {
			h.internalError(w, r, "generate verification code", genErr)
			return
		}
		if storeErr := h.store.Users.SetVerificationCode(r.Context(), email, code, verificationCodeTTL); storeErr != nil {
			h.internalError(w, r, "store verification code", storeErr)
			return
		}
		h.dispatchVerificationEmail(user.Email, user.Name, code)
	}
	httphelper.JSON(w, http.StatusAccepted, map[string]string{
		"message": "if this address is unverified, a new code is on the way",
	})
}

// dispatchVerificationEmail sends the code from a goroutine so registration
// never waits on the mail provider. Delivery failures are logged; users can
// recover through the resend endpoint.
func (h *Handler) dispatchVerificationEmail(email, name, code string) {
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		err := mailhelper.ErrNotConfigured
		if h.mail != nil {
			err = h.mail.SendVerification(ctx, mailhelper.VerificationRequest{
				ToEmail: email,
				ToName:  name,
				Code:    code,
				TTL:     verificationCodeTTL,
				SiteURL: h.publicScheme + "://" + h.publicHost,
			})
		}
		switch {
		case errors.Is(err, mailhelper.ErrNotConfigured):
			h.logger.Warn("email delivery is not configured; verification code", "email", email, "code", code)
		case err != nil:
			h.logger.Error("send verification email", "error", err, "email", email)
		default:
			h.logger.Info("verification email sent", "email", email)
		}
	}()
}

// parseVerificationEmail validates the address before any store access.
func parseVerificationEmail(w http.ResponseWriter, value string) (string, bool) {
	email := strings.TrimSpace(value)
	address, err := mail.ParseAddress(email)
	if err != nil || address.Address != email {
		httphelper.Error(w, http.StatusBadRequest, "valid email is required")
		return "", false
	}
	return strings.ToLower(email), true
}
