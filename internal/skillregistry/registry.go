package skillregistry

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/FeelPulse/feelpulse/internal/logger"
)

const (
	defaultRegistry = "https://clawhub.ai"
	cacheTTL        = 7 * 24 * time.Hour // 7 days
	maxSkills       = 200                 // fetch up to 200 skills
)

// SkillMeta represents a skill's metadata from ClaWHub
type SkillMeta struct {
	Slug      string `json:"slug"`
	Summary   string `json:"summary"`
	Version   string `json:"version"`
	UpdatedAt int64  `json:"updatedAt"`
}

// Registry manages skill metadata from ClaWHub
type Registry struct {
	Items     []SkillMeta `json:"items"`
	CachedAt  time.Time   `json:"cachedAt"`
	cachePath string
	log       *logger.Logger
}

// apiResponse matches ClaWHub /api/v1/skills response
type apiResponse struct {
	Items []apiSkill `json:"items"`
}

type apiSkill struct {
	Slug          string        `json:"slug"`
	Summary       string        `json:"summary"`
	UpdatedAt     int64         `json:"updatedAt"`
	LatestVersion *apiVersion   `json:"latestVersion"`
}

type apiVersion struct {
	Version string `json:"version"`
}

// NewRegistry creates a new skill registry with cache at the given path
func NewRegistry(cachePath string) *Registry {
	return &Registry{
		Items:     []SkillMeta{},
		cachePath: cachePath,
		log:       logger.New(&logger.Config{Level: "info", Component: "skillregistry"}),
	}
}

// Load loads the registry from cache or fetches from API if stale
func (r *Registry) Load() error {
	// Try loading from cache first
	if err := r.loadFromCache(); err == nil {
		// Check if cache is still valid
		if time.Since(r.CachedAt) < cacheTTL {
			r.log.Debug("📦 Skill registry loaded from cache (%d skills, age: %s)", 
				len(r.Items), time.Since(r.CachedAt).Round(time.Hour))
			return nil
		}
		r.log.Debug("⏰ Skill registry cache expired (age: %s), refreshing...", 
			time.Since(r.CachedAt).Round(time.Hour))
	}

	// Cache miss or stale - fetch from API
	if err := r.fetchFromAPI(); err != nil {
		r.log.Warn("Failed to fetch skills from ClaWHub: %v", err)
		// If we have stale cache, use it as fallback
		if len(r.Items) > 0 {
			r.log.Info("📦 Using stale skill registry cache (%d skills)", len(r.Items))
			return nil
		}
		return fmt.Errorf("failed to load skill registry: %w", err)
	}

	// Save to cache
	if err := r.saveToCache(); err != nil {
		r.log.Warn("Failed to save skill registry cache: %v", err)
	}

	return nil
}

// fetchFromAPI fetches skills from ClaWHub API
func (r *Registry) fetchFromAPI() error {
	url := fmt.Sprintf("%s/api/v1/skills?limit=%d&sort=updated", defaultRegistry, maxSkills)
	
	r.log.Debug("🌐 Fetching skills from %s", url)
	
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("API returned status %d", resp.StatusCode)
	}

	var apiResp apiResponse
	if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
		return fmt.Errorf("failed to decode API response: %w", err)
	}

	// Convert to our format
	r.Items = make([]SkillMeta, 0, len(apiResp.Items))
	for _, item := range apiResp.Items {
		version := ""
		if item.LatestVersion != nil {
			version = item.LatestVersion.Version
		}
		r.Items = append(r.Items, SkillMeta{
			Slug:      item.Slug,
			Summary:   item.Summary,
			Version:   version,
			UpdatedAt: item.UpdatedAt,
		})
	}

	r.CachedAt = time.Now()
	r.log.Info("📦 Fetched %d skills from ClaWHub", len(r.Items))
	
	return nil
}

// loadFromCache loads the registry from disk cache
func (r *Registry) loadFromCache() error {
	data, err := os.ReadFile(r.cachePath)
	if err != nil {
		return err
	}

	if err := json.Unmarshal(data, r); err != nil {
		return err
	}

	return nil
}

// saveToCache saves the registry to disk cache
func (r *Registry) saveToCache() error {
	// Ensure parent directory exists
	dir := filepath.Dir(r.cachePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	data, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(r.cachePath, data, 0644)
}

// GetAll returns all skill metadata
func (r *Registry) GetAll() []SkillMeta {
	return r.Items
}

// Find returns a skill by slug, or nil if not found
func (r *Registry) Find(slug string) *SkillMeta {
	for _, item := range r.Items {
		if item.Slug == slug {
			return &item
		}
	}
	return nil
}

// BuildSkillListPrompt builds the skill list section for system prompt
func (r *Registry) BuildSkillListPrompt() string {
	if len(r.Items) == 0 {
		return ""
	}

	prompt := fmt.Sprintf(`

## Available Skills from ClaWHub

%d+ specialized CLI tools available via read_skill.
Examples: github, docker, weather, postgres, k8s, aws, terraform, redis, etc.

**When to use read_skill:**
- Platform-specific operations (GitHub, AWS, Docker, etc.) → read_skill FIRST
- Before using any specialized CLI tool → check if a skill exists
- If skill not installed → I'll ask user for permission to install

Skills provide step-by-step commands and best practices.`, len(r.Items))

	return prompt
}
