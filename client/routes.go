package client

import (
	"net/http"

	httpSwagger "github.com/swaggo/http-swagger"

	"github.com/JspBack/webfp-drive/client/handlers"
	apidocs "github.com/JspBack/webfp-drive/docs"
)

func (c *Client) registerRoutes(mux *http.ServeMux) {
	h := handlers.New(c.Store, c.log, c.MaxFileSize, c.MaxRequestBodyBytes, c.publicHost, c.UseTLS())
	mux.Handle("GET /swagger", http.RedirectHandler("/swagger/index.html", http.StatusTemporaryRedirect))
	mux.Handle("GET /swagger/", httpSwagger.Handler(
		httpSwagger.URL("/swagger/doc.json"),
		httpSwagger.InstanceName(apidocs.SwaggerInfo.InstanceName()),
		httpSwagger.UIConfig(map[string]string{
			"requestInterceptor": `(request) => {
				const token = request.headers.Authorization;
				if (token && !/^Bearer\s/i.test(token)) {
					request.headers.Authorization = "Bearer " + token;
				}
				return request;
			}`,
		}),
	))
	mux.HandleFunc("GET /api/health", h.Health)
	mux.HandleFunc("POST /api/v1/auth/register", h.Register)
	mux.HandleFunc("POST /api/v1/auth/login", h.Login)
	mux.HandleFunc("POST /api/v1/auth/refresh", h.Refresh)
	mux.Handle("POST /api/v1/auth/logout", h.RequireSession(http.HandlerFunc(h.Logout)))
	mux.Handle("GET /api/v1/users/me", h.RequireSession(http.HandlerFunc(h.GetUser)))
	mux.Handle("PATCH /api/v1/users/me", h.RequireSession(http.HandlerFunc(h.UpdateUser)))
	mux.Handle("POST /api/v1/files", h.RequireSession(http.HandlerFunc(h.CreateFile)))
	mux.Handle("GET /api/v1/files", h.RequireSession(http.HandlerFunc(h.ListFiles)))
	mux.Handle("GET /api/v1/files/{id}", h.RequireSession(http.HandlerFunc(h.GetFile)))
	mux.Handle("GET /api/v1/files/{id}/download", h.RequireSession(http.HandlerFunc(h.DownloadFile)))
	mux.Handle("PATCH /api/v1/files/{id}", h.RequireSession(http.HandlerFunc(h.UpdateFile)))
	mux.Handle("DELETE /api/v1/files/{id}", h.RequireSession(http.HandlerFunc(h.DeleteFile)))
	mux.Handle("POST /api/v1/folders", h.RequireSession(http.HandlerFunc(h.CreateFolder)))
	mux.Handle("GET /api/v1/folders", h.RequireSession(http.HandlerFunc(h.ListFolders)))
	mux.Handle("GET /api/v1/folders/{id}", h.RequireSession(http.HandlerFunc(h.GetFolder)))
	mux.Handle("PATCH /api/v1/folders/{id}", h.RequireSession(http.HandlerFunc(h.UpdateFolder)))
	mux.Handle("DELETE /api/v1/folders/{id}", h.RequireSession(http.HandlerFunc(h.DeleteFolder)))
	mux.Handle("POST /api/v1/files/{id}/shares", h.RequireSession(http.HandlerFunc(h.CreateShare)))
	mux.Handle("DELETE /api/v1/shares/{id}", h.RequireSession(http.HandlerFunc(h.RevokeShare)))
	mux.HandleFunc("GET /api/v1/shares/{token}", h.DownloadSharedFile)
	mux.HandleFunc("POST /api/v1/shares/{token}/download", h.SubmitSharePassword)
}
