package tcpguard

import (
	"context"
	"net/http"
	"strings"

	"gymbag/internal/config"
	"gymbag/internal/store"
	"gymbag/internal/waf"
)

// ConnInfo captures client connection metadata for SSH and Minecraft policy evaluation.
type ConnInfo struct {
	Service         string // "ssh" or "minecraft"
	ClientIP        string
	ClientBanner    string // SSH client version string or Minecraft serverAddress
	Username        string // Minecraft username
	ProtoSuspicious bool
	ProtoAI         bool
}

// ServicePolicy describes the protection settings for a TCP service (SSH or Minecraft).
type ServicePolicy struct {
	Protect      bool
	VerifyLevel  config.VerifyLevel
	Difficulty   int
	ChallengeTTL int
	ClearanceTTL int
	RateLimit    int
	RateWindow   int
	Rules        []config.CustomRule
}

// PolicyEngine evaluates custom rules, rate limits, IP threat/VPN/AI intel, IP clearances,
// and verify levels (0-3) for non-HTTP services (SSH and Minecraft).
type PolicyEngine struct {
	service          string
	policy           ServicePolicy
	rulesRule        *waf.CustomRulesRule
	rateCounter      store.RateCounter
	gateStore        store.GateStore
	intel            waf.IntelProvider
	ipSecurityAction string
}

// NewPolicyEngine constructs a PolicyEngine combining global and per-service custom rules.
func NewPolicyEngine(
	service string,
	policy ServicePolicy,
	globalRules []config.CustomRule,
	rateCounter store.RateCounter,
	gateStore store.GateStore,
	intel waf.IntelProvider,
	ipSecurityAction string,
) *PolicyEngine {
	combined := make([]config.CustomRule, 0, len(globalRules)+len(policy.Rules))
	combined = append(combined, globalRules...)
	combined = append(combined, policy.Rules...)

	var rulesRule *waf.CustomRulesRule
	if len(combined) > 0 {
		rulesRule = waf.NewCustomRulesRule(combined, nil, "")
	}
	act := strings.ToLower(strings.TrimSpace(ipSecurityAction))
	if act != "block" {
		act = "verify"
	}
	return &PolicyEngine{
		service:          service,
		policy:           policy,
		rulesRule:        rulesRule,
		rateCounter:      rateCounter,
		gateStore:        gateStore,
		intel:            intel,
		ipSecurityAction: act,
	}
}

// HasBannerRules reports whether custom rules inspect UserAgent/banner fields.
func (e *PolicyEngine) HasBannerRules() bool {
	if e == nil {
		return false
	}
	return e.rulesRule != nil
}

// Policy returns the active ServicePolicy snapshot.
func (e *PolicyEngine) Policy() ServicePolicy {
	return e.policy
}

// IsFastAllow checks if a connection can be relayed immediately without peeking protocol banners
// (e.g., !Protect or already holding an active IP clearance with no blocking rules).
func (e *PolicyEngine) IsFastAllow(ctx context.Context, clientIP string) (allowed bool, decided bool, decision waf.Decision) {
	if e == nil {
		return true, true, waf.Allow()
	}
	// If custom rules exist, evaluate IP-only first when there's only IP rules?
	// Safer: only fast-allow without banner peek if there are no custom rules AND either !Protect or cleared.
	if e.rulesRule != nil {
		return false, false, waf.Decision{}
	}
	if !e.policy.Protect {
		return true, true, waf.Allow()
	}
	if e.gateStore != nil {
		if _, cleared, err := e.gateStore.IsCleared(ctx, e.service, clientIP); err == nil && cleared {
			return true, true, waf.Allow()
		}
	}
	return false, false, waf.Decision{}
}

// Evaluate applies the Gymbag policy pipeline to info and returns Allow, Challenge, or Deny.
func (e *PolicyEngine) Evaluate(ctx context.Context, info ConnInfo) waf.Decision {
	if e == nil {
		return waf.Allow()
	}

	// 1. Custom rules (global + per-service, first match wins).
	if e.rulesRule != nil {
		banner := strings.TrimSpace(info.ClientBanner)
		if banner == "" {
			banner = strings.TrimSpace(info.Username)
		} else if info.Username != "" {
			banner = banner + " " + info.Username
		}
		inc := waf.IncomingRequest{
			Method:   "TCP",
			Host:     info.Service,
			Path:     "/",
			ClientIP: info.ClientIP,
			Headers: map[string]string{
				"user-agent": banner,
			},
		}
		if dec := e.rulesRule.Evaluate(ctx, inc); dec != nil {
			switch dec.Action {
			case waf.ActionAllow, waf.ActionDeny:
				return *dec
			case waf.ActionChallenge:
				if !e.policy.Protect {
					return waf.Allow()
				}
				if e.isCleared(ctx, info.ClientIP) {
					return waf.Allow()
				}
				return waf.Challenge()
			}
		}
	}

	// 2. Unprotected mode bypasses rate limit, threat challenge, and PoW verification.
	if !e.policy.Protect {
		return waf.Allow()
	}

	// 3. Per-service per-IP rate limiter.
	if e.rateCounter != nil && e.policy.RateLimit > 0 && e.policy.RateWindow > 0 {
		key := "tcp:" + e.service + ":" + info.ClientIP
		if count, err := e.rateCounter.Increment(ctx, key, e.policy.RateWindow); err == nil && count > e.policy.RateLimit {
			return waf.Deny("rate", http.StatusTooManyRequests)
		}
	}

	// 4. Threat intelligence lookup.
	isThreat := false
	if e.intel != nil && e.intel.IsThreatIP(ctx, info.ClientIP) {
		isThreat = true
		if e.ipSecurityAction == "block" {
			return waf.Deny("threat", http.StatusForbidden)
		}
	}

	// 5. Cleared IP check (browser PoW already passed for this service + IP).
	if e.isCleared(ctx, info.ClientIP) {
		return waf.Allow()
	}

	if isThreat {
		return waf.Challenge()
	}

	// 6. Verify level evaluation (0=always, 1=only suspicious, 2=only ai, 3=suspicious & ai).
	switch e.policy.VerifyLevel {
	case config.VerifyLevelAlways:
		return waf.Challenge()
	case config.VerifyLevelOnlySuspicious:
		if e.isSuspicious(ctx, info) {
			return waf.Challenge()
		}
		return waf.Allow()
	case config.VerifyLevelOnlyAI:
		if e.isAI(info) {
			return waf.Challenge()
		}
		return waf.Allow()
	case config.VerifyLevelSuspiciousAndAI:
		if e.isSuspicious(ctx, info) || e.isAI(info) {
			return waf.Challenge()
		}
		return waf.Allow()
	default:
		return waf.Challenge()
	}
}

func (e *PolicyEngine) isCleared(ctx context.Context, clientIP string) bool {
	if e.gateStore == nil {
		return false
	}
	_, cleared, err := e.gateStore.IsCleared(ctx, e.service, clientIP)
	return err == nil && cleared
}

func (e *PolicyEngine) isSuspicious(ctx context.Context, info ConnInfo) bool {
	if info.ProtoSuspicious {
		return true
	}
	if e.intel != nil {
		if vp, ok := e.intel.(waf.VPNIntelProvider); ok {
			if vp.IsVPNOrProxyIP(ctx, info.ClientIP) {
				return true
			}
		}
	}
	return false
}

func (e *PolicyEngine) isAI(info ConnInfo) bool {
	if info.ProtoAI {
		return true
	}
	if e.intel != nil && e.intel.IsAIIP(info.ClientIP) {
		return true
	}
	return false
}
