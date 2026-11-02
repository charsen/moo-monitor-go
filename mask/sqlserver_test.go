package mask

import (
	"strings"
	"testing"
)

func TestSQLServerInsertOutput(t *testing.T) {
	masker, err := New([]string{"token"})
	if err != nil {
		t.Fatal(err)
	}
	for _, sql := range []string{
		`INSERT INTO "items" ("token") OUTPUT INSERTED."id" VALUES ('fixed-secret');`,
		`INSERT INTO items ([token],id) OUTPUT INSERTED.[id], INSERTED."created_at" VALUES ('fixed-secret',1),('second-secret',2);`,
		`insert into items (id, token) output inserted.id values (1,'fixed-secret');`,
	} {
		got := masker.MaskSensitiveSQL(sql)
		if strings.Contains(got, "secret") || !strings.Contains(got, "***") || !strings.Contains(strings.ToLower(got), "output inserted.") {
			t.Fatal("OUTPUT insert was not safely masked")
		}
	}
	// Unsupported OUTPUT expressions are not silently treated as the known column list.
	for _, sql := range []string{
		`INSERT INTO items (token) OUTPUT inserted.id, abs(inserted.id) VALUES ('fixed');`,
		`INSERT INTO items (token) OUTPUT (SELECT id FROM other) VALUES ('fixed');`,
		`INSERT INTO items (token) OUTPUT inserted.id AS other VALUES ('fixed');`,
	} {
		if got := masker.MaskSensitiveSQL(sql); got != sql {
			t.Fatal("unknown OUTPUT grammar accepted")
		}
	}
}
