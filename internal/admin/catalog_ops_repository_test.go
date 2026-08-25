package admin

import (
	"strings"
	"testing"
)

// A strategy is identified by (strategy_id, strategy_version), never by the
// version alone: version strings are not globally unique, so "model-a/v1" and
// "model-b/v1" are different embeddings that happen to share a label. If the
// stale filter compared versions only, switching between two same-versioned
// strategies would reset nothing and the re-embed would close as an instant
// success over vectors from the old model.
func TestReembedResetSQL_StaleFilterComparesTheWholeTuple(t *testing.T) {
	sql := reembedResetSQL("")

	if !strings.Contains(sql, "(strategy_id, strategy_version) <> ($2, $3)") {
		t.Errorf("stale filter must compare the whole strategy identity:\n%s", sql)
	}
	if strings.Contains(sql, "strategy_version <> $2") {
		t.Errorf("version-only comparison must not survive:\n%s", sql)
	}
}

// A row that has never been embedded has a NULL identity. It is stale by
// definition — excluding NULLs would leave freshly-ingested items out of a
// re-embed that is supposed to cover the namespace.
func TestReembedResetSQL_UnembeddedRowsCountAsStale(t *testing.T) {
	sql := reembedResetSQL("")

	if !strings.Contains(sql, "strategy_id IS NULL OR strategy_version IS NULL") {
		t.Errorf("NULL identities must be treated as stale:\n%s", sql)
	}
}

// Naming a state means "re-drive these rows regardless of which strategy
// produced them" — the rebuild-after-Qdrant-loss path. The version filter has
// to drop out entirely, or a rebuild at the current strategy would match
// nothing.
func TestReembedResetSQL_ExplicitStateDropsTheStrategyFilter(t *testing.T) {
	for _, onlyState := range []string{ReembedOnlyStateAll, ReembedOnlyStateEmbedded, ReembedOnlyStateFailed} {
		sql := reembedResetSQL(onlyState)
		if strings.Contains(sql, "(strategy_id, strategy_version)") {
			t.Errorf("only_state=%q must re-drive regardless of strategy:\n%s", onlyState, sql)
		}
	}
}

// The reset nulls the previous identity so a row cannot be mistaken for
// already-embedded at the new target while it is waiting to be re-embedded.
func TestReembedResetSQL_ClearsThePreviousIdentity(t *testing.T) {
	for _, onlyState := range []string{"", ReembedOnlyStateAll, ReembedOnlyStateEmbedded, ReembedOnlyStateFailed, "bogus"} {
		sql := reembedResetSQL(onlyState)
		if !strings.Contains(sql, "strategy_version = NULL") {
			t.Errorf("only_state=%q must invalidate the previous strategy version:\n%s", onlyState, sql)
		}
		if !strings.Contains(sql, "state = 'pending'") {
			t.Errorf("only_state=%q must return rows to pending:\n%s", onlyState, sql)
		}
	}
}

// onlyState is caller-validated, but the builder must not be the thing that
// trusts it: an unrecognised value falls back to the stale-only default and
// never reaches the SQL text.
func TestReembedResetSQL_UnknownStateNeverReachesTheQuery(t *testing.T) {
	sql := reembedResetSQL("'; DROP TABLE catalog_items; --")

	if strings.Contains(sql, "DROP TABLE") {
		t.Fatalf("caller input reached the SQL text:\n%s", sql)
	}
	if !strings.Contains(sql, "(strategy_id, strategy_version) <> ($2, $3)") {
		t.Errorf("unknown state must fall back to the stale-only default:\n%s", sql)
	}
}

// The target is read back from the run's own columns, so a completion check
// judges against what the run was started with rather than whatever the
// namespace is configured for now.
func TestReembedTargetFromBatchRow(t *testing.T) {
	id, version := "model-a", "v2"
	got, gotVersion := ReembedTargetFromBatchRow(&BatchRunLog{
		TargetStrategyID:      &id,
		TargetStrategyVersion: &version,
	})
	if got != "model-a" || gotVersion != "v2" {
		t.Errorf("got (%q, %q), want (model-a, v2)", got, gotVersion)
	}

	// Rows written before migration 012 carry NULL targets; they must read
	// back as empty rather than panic.
	if got, gotVersion = ReembedTargetFromBatchRow(&BatchRunLog{}); got != "" || gotVersion != "" {
		t.Errorf("pre-012 row: got (%q, %q), want empty", got, gotVersion)
	}
	if got, gotVersion = ReembedTargetFromBatchRow(nil); got != "" || gotVersion != "" {
		t.Errorf("nil row: got (%q, %q), want empty", got, gotVersion)
	}
}
