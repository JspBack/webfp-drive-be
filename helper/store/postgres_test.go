package storehelper_test

import (
	"fmt"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"

	storehelper "github.com/JspBack/webfp-drive/helper/store"
)

func TestIsForeignKeyViolation(t *testing.T) {
	err := fmt.Errorf("query failed: %w", &pgconn.PgError{
		Code:           "23503",
		ConstraintName: "files_folder_owner_fk",
	})
	if !storehelper.IsForeignKeyViolation(err, "files_folder_owner_fk") {
		t.Fatal("expected matching foreign key violation")
	}
	if storehelper.IsForeignKeyViolation(err, "folders_parent_owner_fk") {
		t.Fatal("unexpected match for a different constraint")
	}
	if storehelper.IsForeignKeyViolation(nil, "files_folder_owner_fk") {
		t.Fatal("nil error matched a foreign key violation")
	}
	if storehelper.IsForeignKeyViolation(&pgconn.PgError{
		Code:           "23505",
		ConstraintName: "files_folder_owner_fk",
	}, "files_folder_owner_fk") {
		t.Fatal("unique violation matched a foreign key violation")
	}
}
