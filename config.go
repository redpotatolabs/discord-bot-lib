package botlib

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"

	"github.com/epistemi/beacon/pkg/beacon"
)

// BaseConfig holds the fields every redpotatolabs Discord bot needs. Callers
// embed it in their own Config struct and add their domain fields
// alongside:
//
//	type Config struct {
//	    botlib.BaseConfig
//	    DataPath string `json:"dataPath"`
//	    // ...domain fields
//	}
type BaseConfig struct {
	Token       string `json:"token"`
	ClientID    string `json:"clientId"`
	GuildID     string `json:"guildId"`
	BeaconURL   string `json:"beaconUrl"`
	BeaconToken string `json:"beaconToken"`
}

// Validate enforces the only field every bot needs: Token. Missing
// ClientID is acceptable (commands won't auto-register; the bot is
// otherwise functional); callers can additionally warn about that
// themselves.
func (c BaseConfig) Validate() error {
	if c.Token == "" {
		return errors.New("config token is required")
	}
	return nil
}

// LoadJSONConfig reads a JSON file from path and decodes it into T. T is
// expected to be a struct (typically one embedding BaseConfig). Callers
// are responsible for validating domain fields after this returns.
func LoadJSONConfig[T any](path string) (T, error) {
	var zero T
	data, err := os.ReadFile(path)
	if err != nil {
		return zero, fmt.Errorf("read config: %w", err)
	}
	var cfg T
	if err := json.Unmarshal(data, &cfg); err != nil {
		return zero, fmt.Errorf("parse config: %w", err)
	}
	return cfg, nil
}

// BeaconFromConfig returns a beacon.Client when both BeaconURL and
// BeaconToken are non-empty, nil otherwise. Saves a 4-line if-block
// repeated across every bot's main.go.
func BeaconFromConfig(c BaseConfig) *beacon.Client {
	if c.BeaconURL == "" || c.BeaconToken == "" {
		return nil
	}
	return beacon.New(c.BeaconURL, c.BeaconToken)
}
