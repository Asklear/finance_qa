package feishusync

import (
	"os"
	"strings"
)

func feishuMetadataShortcutEnabled() bool {
	if envTruthy(os.Getenv("FINANCEQA_FEISHU_FORCE_DOWNLOAD")) {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(os.Getenv("FINANCEQA_FEISHU_METADATA_SHORTCUT"))) {
	case "0", "false", "off", "no", "disabled":
		return false
	default:
		return true
	}
}

func envTruthy(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "true", "yes", "on", "force":
		return true
	default:
		return false
	}
}
