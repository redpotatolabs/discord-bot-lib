package botlib

import (
	"context"
	"fmt"

	"github.com/bwmarrin/discordgo"
	"github.com/redpotatolabs/beacon/pkg/beacon"
)

// CommandRegistrar is everything the dispatcher needs from the caller.
// Each bot implements this in its own commands package (typically thin
// shims over the existing commands.Definitions / commands.Handle).
type CommandRegistrar interface {
	// Definitions returns the application command definitions to install
	// per guild.
	Definitions() []*discordgo.ApplicationCommand

	// Handle dispatches an InteractionApplicationCommand. Returns
	// (handled, err). If handled is true and err is non-nil, the
	// dispatcher sends an ephemeral "error executing command" reply.
	Handle(s *discordgo.Session, i *discordgo.InteractionCreate) (bool, error)

	// HandleAutocomplete dispatches an
	// InteractionApplicationCommandAutocomplete.
	HandleAutocomplete(s *discordgo.Session, i *discordgo.InteractionCreate) (bool, error)
}

// GuildLister discovers which guilds to register commands for. Each bot
// has a store that already exposes ListGuildIDs(); satisfy this interface
// with that.
type GuildLister interface {
	ListGuildIDs() ([]string, error)
}

// RegisterDispatch wires Ready, GuildCreate, and InteractionCreate
// handlers onto the App's session.
//
//   - Ready: clears global application commands (we register per-guild
//     for instant propagation) and registers per known guild.
//   - GuildCreate: registers commands for the newly-joined guild.
//   - InteractionCreate: dispatches application commands and
//     autocomplete. Panics are recovered, reported through
//     App.Reporter, and surfaced as an ephemeral error reply.
//
// fallbackGuildID is used only if guilds.ListGuildIDs() returns empty.
// clientID may be "" — in that case automatic command registration is
// skipped and the dispatcher logs a warning at Ready.
func (a *App) RegisterDispatch(registrar CommandRegistrar, guilds GuildLister, clientID, fallbackGuildID string) {
	s := a.Session
	log := a.logger

	s.AddHandler(func(s *discordgo.Session, _ *discordgo.Ready) {
		log.Infof("ready as %s", s.State.User.String())
		registerCommandsAtReady(s, log, registrar, guilds, clientID, fallbackGuildID)
	})

	s.AddHandler(func(s *discordgo.Session, g *discordgo.GuildCreate) {
		if clientID == "" {
			return
		}
		if _, err := s.ApplicationCommandBulkOverwrite(clientID, g.ID, registrar.Definitions()); err != nil {
			log.Warnf("register commands for guild %s on GuildCreate: %v", g.ID, err)
			return
		}
		log.Infof("registered commands for guild %s (%s) via GuildCreate", g.Name, g.ID)
	})

	s.AddHandler(func(s *discordgo.Session, i *discordgo.InteractionCreate) {
		defer func() {
			rec := recover()
			if rec == nil {
				return
			}
			fields := []beacon.FieldKV{
				{Name: "interaction_type", Value: i.Type.String(), Inline: true},
			}
			if i.GuildID != "" {
				fields = append(fields, beacon.FieldKV{Name: "guild_id", Value: i.GuildID, Inline: true})
			}
			a.Reporter.Report(context.Background(), "fatal",
				fmt.Sprintf("panic in interaction handler: %v", rec),
				fields...)
		}()
		switch i.Type {
		case discordgo.InteractionApplicationCommand:
			if handled, err := registrar.Handle(s, i); handled {
				if err != nil {
					log.Warnf("command interaction: %v", err)
					RespondEphemeral(s, i.Interaction, "There was an error while executing this command.")
				}
			}
		case discordgo.InteractionApplicationCommandAutocomplete:
			if _, err := registrar.HandleAutocomplete(s, i); err != nil {
				log.Warnf("autocomplete: %v", err)
			}
		}
	})
}

func registerCommandsAtReady(s *discordgo.Session, log Logger, registrar CommandRegistrar, guilds GuildLister, clientID, fallbackGuildID string) {
	if clientID == "" {
		log.Warnf("command registration skipped: clientId is empty")
		return
	}
	defs := registrar.Definitions()
	// Clear any global application commands. We register per-guild
	// for instant propagation; stale globals would show up as
	// duplicates in every guild. Idempotent after the first run.
	if _, err := s.ApplicationCommandBulkOverwrite(clientID, "", nil); err != nil {
		log.Warnf("clear global commands: %v", err)
	}
	guildIDs, err := guilds.ListGuildIDs()
	if err != nil {
		log.Warnf("load known guilds: %v", err)
	}
	if len(guildIDs) == 0 && fallbackGuildID != "" {
		guildIDs = []string{fallbackGuildID}
		log.Infof("no guilds in store, using fallback guild %s", fallbackGuildID)
	}
	if len(guildIDs) == 0 {
		log.Infof("no guilds to register commands against; will register on first GuildCreate")
	}
	for _, gid := range guildIDs {
		if _, err := s.ApplicationCommandBulkOverwrite(clientID, gid, defs); err != nil {
			log.Warnf("register commands for guild %s: %v", gid, err)
			continue
		}
		log.Infof("registered commands for guild %s", gid)
	}
}
