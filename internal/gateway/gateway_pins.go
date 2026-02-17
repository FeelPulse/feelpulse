package gateway

import (
	"crypto/rand"
	"fmt"
	"strings"
	"time"

	"github.com/FeelPulse/feelpulse/internal/command"
	"github.com/FeelPulse/feelpulse/internal/logger"
	"github.com/FeelPulse/feelpulse/internal/store"
)

// pinManager implements command.PinProvider using SQLite
type pinManager struct {
	db  *store.SQLiteStore
	log *logger.Logger
}

func newPinManager(db *store.SQLiteStore, log *logger.Logger) (*pinManager, error) {
	if db == nil {
		return nil, fmt.Errorf("database not available")
	}
	if err := db.EnsurePinsTable(); err != nil {
		return nil, fmt.Errorf("failed to create pins table: %w", err)
	}
	return &pinManager{db: db, log: log}, nil
}

func (pm *pinManager) AddPin(sessionKey, text string) (string, error) {
	// Generate unique ID
	b := make([]byte, 8)
	rand.Read(b)
	id := fmt.Sprintf("pin-%x", b)

	pin := &store.PinData{
		ID:         id,
		SessionKey: sessionKey,
		Text:       text,
		CreatedAt:  time.Now(),
	}

	if err := pm.db.SavePin(pin); err != nil {
		return "", err
	}

	pm.log.Debug("📌 Pin created: %s for session %s", id, sessionKey)
	return id, nil
}

func (pm *pinManager) ListPins(sessionKey string) []command.PinInfo {
	pins, err := pm.db.LoadPinsBySession(sessionKey)
	if err != nil {
		pm.log.Warn("Failed to load pins: %v", err)
		return nil
	}

	result := make([]command.PinInfo, len(pins))
	for i, p := range pins {
		result[i] = command.PinInfo{
			ID:        p.ID,
			Text:      p.Text,
			CreatedAt: p.CreatedAt,
		}
	}
	return result
}

func (pm *pinManager) RemovePin(id string) error {
	if err := pm.db.DeletePin(id); err != nil {
		return err
	}
	pm.log.Debug("📌 Pin deleted: %s", id)
	return nil
}

func (pm *pinManager) GetPins(sessionKey string) string {
	pins, err := pm.db.LoadPinsBySession(sessionKey)
	if err != nil || len(pins) == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("\n\n[USER PINNED INFORMATION - Always consider this context]\n")
	for _, p := range pins {
		sb.WriteString(fmt.Sprintf("- %s\n", p.Text))
	}
	return sb.String()
}