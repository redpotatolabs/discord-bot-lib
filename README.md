# discord-bot-lib

The shared lifecycle harness for the Red Potato Labs Discord bots, built on [discordgo](https://github.com/bwmarrin/discordgo). It owns the session, slash command registration and dispatch, background workers, a gateway watchdog, graceful shutdown and error reporting through [beacon-client](https://github.com/redpotatolabs/beacon-client). Domain code, such as commands, stores and business logic, stays in each bot.

## Install

```sh
go get github.com/redpotatolabs/discord-bot-lib@latest
```

## Use

```go
import (
    botlib "github.com/redpotatolabs/discord-bot-lib"
    "github.com/redpotatolabs/discord-bot-lib/zaplog"
)

cfg, err := botlib.LoadJSONConfig[Config](path)

app, err := botlib.New(cfg.Token, discordgo.IntentsGuilds)
log, _ := zaplog.New()
app.SetLogger(log)
app.SetBeacon(botlib.BeaconFromConfig(cfg.BaseConfig))

app.RegisterDispatch(myCommands, myGuilds, cfg.ClientID, cfg.GuildID)

err = app.Run(ctx, botlib.RunConfig{
    Workers: []botlib.Worker{reminderLoop},
})
```

## What ships

| Symbol | Purpose |
| --- | --- |
| `App`, `New`, `App.Run`, `RunConfig` | session lifecycle: open, run workers, watch the gateway, shut down |
| `Worker`, `Shutdowner` | background tasks and things to close on shutdown |
| `CommandRegistrar`, `GuildLister`, `App.RegisterDispatch` | slash command registration and interaction dispatch |
| `BaseConfig`, `LoadJSONConfig[T]`, `BeaconFromConfig` | the config fields every bot needs, loaded from JSON |
| `Logger`, `StdLogger` | small logging interface with a stdlib default |
| `App.SetBeacon`, `BeaconReporter` | error reporting that logs and, when configured, posts to beacon |
| `ErrGatewayStalled` | returned by `Run` when the watchdog finds the gateway dead |
| `zaplog.New`, `zaplog.Logger` | zap-backed `Logger` in its own package, so bots without zap do not pull it in |

The watchdog exists because of a discordgo deadlock on gateway reconnect: a stalled session is detected and `Run` returns, so the process supervisor restarts the bot instead of it hanging silently.

## License

MIT, see [LICENSE](LICENSE).
