// Package loader reads the generator's JSON output back into Go structs
// so the matcher can operate on it independently of how it was produced.
package loader

import (
	"encoding/json"
	"os"
	"path/filepath"

	"threefolds/internal/model"
)

// Data bundles everything read back from disk.
type Data struct {
	Settlements    []model.Settlement
	BankStatements []model.BankStatement
	LedgerEntries  []model.LedgerEntry
}

// Load reads settlements.json, bank_statements.json, and ledger_entries.json
// from dir.
func Load(dir string) (Data, error) {
	var d Data

	if err := readJSON(filepath.Join(dir, "settlements.json"), &d.Settlements); err != nil {
		return d, err
	}
	if err := readJSON(filepath.Join(dir, "bank_statements.json"), &d.BankStatements); err != nil {
		return d, err
	}
	if err := readJSON(filepath.Join(dir, "ledger_entries.json"), &d.LedgerEntries); err != nil {
		return d, err
	}

	return d, nil
}

func readJSON(path string, v interface{}) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return json.NewDecoder(f).Decode(v)
}
