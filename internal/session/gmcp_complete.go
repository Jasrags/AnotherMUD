package session

import (
	"context"
	"encoding/json"
	"log/slog"

	"github.com/Jasrags/AnotherMUD/internal/command"
	"github.com/Jasrags/AnotherMUD/internal/conn"
	"github.com/Jasrags/AnotherMUD/internal/gmcp"
	"github.com/Jasrags/AnotherMUD/internal/logging"
)

// commandEnv builds the per-command command.Env from cfg. Every field is
// cfg-derived (no per-line data), so pump and the inbound GMCP handler
// share one builder.
func commandEnv(cfg Config) command.Env {
	return command.Env{
		World:                 cfg.World,
		Broadcaster:           cfg.Manager,
		Items:                 cfg.Items,
		Placement:             cfg.Placement,
		Contents:              cfg.Contents,
		Slots:                 cfg.Slots,
		Bus:                   cfg.Bus,
		Properties:            cfg.Properties,
		Rarity:                cfg.Rarity,
		Essence:               cfg.Essence,
		Stacking:              cfg.Stacking,
		Locator:               managerLocator{cfg.Manager},
		Roster:                NewWhoRoster(cfg.Manager, cfg.Clock, DefaultWhoConfig()),
		BadInput:              cfg.BadInput,
		Disposition:           cfg.Disposition,
		Combat:                cfg.Combat,
		Flee:                  cfg.Flee,
		ResolveAttack:         cfg.ResolveAttack,
		ReloadScripts:         cfg.ReloadScripts,
		Progression:           cfg.Progression,
		Faction:               cfg.Faction,
		Effects:               cfg.Effects,
		EffectTemplates:       cfg.EffectTemplates,
		SkillRoller:           cfg.SkillRoller,
		RecognitionDifficulty: cfg.RecognitionDifficulty,
		Training:              cfg.Training,
		Abilities:             cfg.Abilities,
		Proficiency:           cfg.Proficiency,
		ActionQueue:           cfg.ActionQueue,
		Recipes:               cfg.Recipes,
		Known:                 cfg.Known,
		Craft:                 cfg.Craft,
		Gathering:             cfg.Gathering,
		Biomes:                cfg.Biomes,
		Grades:                cfg.Grades,
		ForageTables:          cfg.ForageTables,
		Help:                  cfg.Help,
		Quests:                cfg.Quests,
		Currency:              cfg.Currency,
		Money:                 cfg.CurrencyLabel,
		Mounts:                cfg.Mounts,
		Transit:               cfg.Transit,
		Hirelings:             cfg.Hirelings,
		Guides:                cfg.Guides,
		HirelingCap:           cfg.HirelingCap,
		Spawn:                 cfg.Spawn,
		RangedFlavor:          cfg.RangedFlavor,
		Trades:                cfg.Trades,
		Auction:               cfg.Auction,
		Shop:                  cfg.Shop,
		Security:              cfg.Security,
		Rest:                  cfg.Rest,
		Consumable:            cfg.Consumable,
		Notifications:         cfg.Notifications,
		TellResolver:          cfg.TellResolver,
		RoleTargetResolver:    cfg.RoleTargets,
		GrantingRole:          cfg.GrantingRole,
		AdminRole:             cfg.AdminRole,
		DefaultXPTrack:        cfg.DefaultXPTrack,
		Announcer:             cfg.Manager,
		PlayerRoom:            PlayerRoomResolver{cfg.Manager},
		ChatRegistry:          cfg.ChatRegistry,
		ChatSubscribers:       cfg.ChatSubscribers,
		ChatScrollbacks:       cfg.ChatScrollbacks,
		Clock:                 cfg.Clock,
		Ambience:              cfg.Ambience,
		WeatherState:          cfg.WeatherState,
		Light:                 cfg.Light,
		NowTick:               cfg.NowTick,
		CorpseOwnershipWindow: cfg.CorpseOwnershipWindow,
		ReloadTicks:           cfg.ReloadTicks,
		DefaultMoveCost:       cfg.DefaultMoveCost,
		Actions:               cfg.Actions,
		DonTicks:              cfg.DonTicks,
		// follow.md: the Manager owns the move-with-leader graph (it implements
		// command.FollowService) and resolves a player id to its live Actor for
		// cross-room follow messaging.
		Follow: cfg.Manager,
		Group:  cfg.Manager,
		ActorByID: func(id string) (command.Actor, bool) {
			if cfg.Manager == nil {
				return nil, false
			}
			a, ok := cfg.Manager.GetByPlayerID(id)
			if !ok {
				return nil, false
			}
			return a, true
		},
	}
}

// installGmcpInbound installs the inbound (client→server) GMCP handler on
// a connection that supports GMCP (telnet/ws). No-op for a transport
// without GMCP. Called from run() once the actor exists. The handler runs
// synchronously on the read goroutine — the GMCP frame is processed inside
// c.Read before pump dispatches the next line — so it touches actor state
// the same way a command handler does (no extra concurrency vs. dispatch).
func installGmcpInbound(c conn.Connection, a *connActor, cfg Config) {
	gc, ok := c.(conn.GmcpConn)
	if !ok {
		return
	}
	gc.SetGmcpHandler(func(ctx context.Context, pkg string, payload []byte) {
		// Per-connection inbound-GMCP rate limit (separate budget from the
		// command flood gate). Over-rate frames are dropped silently — the
		// completion assist is best-effort, and abuse only sheds GMCP, never
		// disconnects (the command channel keeps its own gate). Applies to
		// every inbound package, so the whole foundation is throttled.
		if a.gmcpFlood != nil {
			if dec, _ := a.gmcpFlood.Check(); dec != floodAllow {
				return
			}
		}
		switch pkg {
		case gmcp.PackageCompleteRequest:
			handleCompleteRequest(ctx, gc, cfg, a, payload)
		default:
			logging.From(ctx).Debug("session: inbound gmcp ignored",
				slog.String("package", pkg))
		}
	})
}

// handleCompleteRequest answers an Input.Complete request: run the
// completion query for this actor on the partial line and reply with
// Input.Complete.List.
func handleCompleteRequest(ctx context.Context, gc conn.GmcpConn, cfg Config, a *connActor, payload []byte) {
	var req gmcp.CompleteRequest
	if err := json.Unmarshal(payload, &req); err != nil {
		logging.From(ctx).Debug("gmcp Input.Complete: bad payload", slog.Any("err", err))
		return
	}
	res := cfg.Commands.CompleteLine(commandEnv(cfg), a, req.Line)
	body, err := json.Marshal(buildCompleteResponse(req.Line, res))
	if err != nil {
		logging.From(ctx).Debug("gmcp Input.Complete: marshal", slog.Any("err", err))
		return
	}
	_ = gc.SendGmcp(ctx, gmcp.PackageCompleteResponse, body)
}

// buildCompleteResponse maps a command.CompletionResult onto the GMCP wire
// shape, computing the longest-common-prefix the client completes to
// (tab-completion §12).
func buildCompleteResponse(line string, res command.CompletionResult) gmcp.CompleteResponse {
	out := gmcp.CompleteResponse{
		Line:       line,
		Target:     completionTargetString(res.Target),
		Verb:       res.Verb,
		Truncated:  res.Truncated,
		Candidates: make([]gmcp.CompleteCandidate, 0, len(res.Candidates)),
	}
	values := make([]string, 0, len(res.Candidates))
	for _, cand := range res.Candidates {
		out.Candidates = append(out.Candidates, gmcp.CompleteCandidate{
			Value:   cand.Completion,
			Display: cand.Display,
			Kind:    string(cand.Kind),
		})
		values = append(values, cand.Completion)
	}
	out.Common = command.LongestCommonPrefix(values)
	return out
}

func completionTargetString(t command.CompletionKind) string {
	switch t {
	case command.CompleteVerb:
		return "verb"
	case command.CompleteArgument:
		return "argument"
	default:
		return "none"
	}
}
