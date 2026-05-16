package doctor

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

func beadsConfigDiagnostic(configPath, beadsDir string) string {
	metadataPath := filepath.Join(beadsDir, "metadata.json")
	data, err := os.ReadFile(metadataPath)
	if err != nil {
		return fmt.Sprintf("config=%s; metadata=%s unreadable: %v", configPath, metadataPath, err)
	}

	var metadata struct {
		Backend      string `json:"backend"`
		DoltMode     string `json:"dolt_mode"`
		DoltDatabase string `json:"dolt_database"`
	}
	if err := json.Unmarshal(data, &metadata); err != nil {
		return fmt.Sprintf("config=%s; metadata=%s invalid: %v", configPath, metadataPath, err)
	}

	return fmt.Sprintf("config=%s; metadata=%s backend=%q dolt_mode=%q dolt_database=%q", configPath, metadataPath, metadata.Backend, metadata.DoltMode, metadata.DoltDatabase)
}
