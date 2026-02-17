package main

import (
	"fmt"
	"runtime"
	"strings"

	"github.com/FeelPulse/feelpulse/internal/config"
)

// Build info - set via ldflags at build time:
//
//	go build -ldflags "-X main.buildTime=$(date -u +%Y-%m-%dT%H:%M:%SZ) -X main.gitCommit=$(git rev-parse --short HEAD)"
var (
	buildTime = "unknown"  // Set via -ldflags "-X main.buildTime=..."
	gitCommit = "unknown"  // Set via -ldflags "-X main.gitCommit=..."
)

// VersionInfo contains version and build information
type VersionInfo struct {
	Version   string
	GoVersion string
	BuildTime string
	GitCommit string
	Platform  string
	Features  []string
}

// GetVersionInfo returns the version information
func GetVersionInfo() *VersionInfo {
	return &VersionInfo{
		Version:   version,
		GoVersion: runtime.Version(),
		BuildTime: buildTime,
		GitCommit: gitCommit,
		Platform:  fmt.Sprintf("%s/%s", runtime.GOOS, runtime.GOARCH),
		Features:  detectEnabledFeatures(),
	}
}

// detectEnabledFeatures checks config for enabled features
func detectEnabledFeatures() []string {
	features := []string{}

	cfg, err := config.Load()
	if err != nil {
		return features
	}

	if cfg.Channels.Telegram.Enabled {
		features = append(features, "telegram")
	}
	if cfg.Browser.Enabled {
		features = append(features, "browser")
	}
	if cfg.TTS.Enabled {
		features = append(features, "tts")
	}
	if cfg.Heartbeat.Enabled {
		features = append(features, "heartbeat")
	}

	return features
}

// String returns formatted version information
func (v *VersionInfo) String() string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("FeelPulse v%s\n", v.Version))
	sb.WriteString(fmt.Sprintf("  Go:       %s\n", v.GoVersion))
	sb.WriteString(fmt.Sprintf("  Platform: %s\n", v.Platform))
	sb.WriteString(fmt.Sprintf("  Build:    %s\n", v.BuildTime))
	sb.WriteString(fmt.Sprintf("  Commit:   %s\n", v.GitCommit))

	if len(v.Features) > 0 {
		sb.WriteString(fmt.Sprintf("  Features: %s\n", strings.Join(v.Features, ", ")))
	} else {
		sb.WriteString("  Features: (none enabled)\n")
	}

	return sb.String()
}

// cmdVersion prints detailed version information
func cmdVersion() {
	info := GetVersionInfo()
	fmt.Print(info.String())
}
