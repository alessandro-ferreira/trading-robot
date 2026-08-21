package execution

import (
	"fmt"
	"math/rand"
	"sync"
	"time"
)

type guardRule struct {
	maxFailures     int
	baseCooldown    time.Duration
	maximumCooldown time.Duration
}

type guardState struct {
	consecutiveFailures int
	penaltyLevel        int
	blockedUntil        time.Time
	probeActive         bool
}

type Guard struct {
	mu     sync.Mutex
	states map[string]*guardState
	rules  map[string]guardRule
}

// GuardError explains why an execution request was blocked before reaching the gateway.
type GuardError struct {
	Key             string
	Remaining       time.Duration
	PenaltyLevel    int
	ProbeInProgress bool
}

func (e *GuardError) Error() string {
	if e.ProbeInProgress {
		return fmt.Sprintf("execution guard blocked %s: recovery probe in progress", e.Key)
	}
	return fmt.Sprintf(
		"execution guard blocked %s: cooldown remaining %s, penalty level %d",
		e.Key,
		e.Remaining.Round(time.Millisecond),
		e.PenaltyLevel,
	)
}

const (
	guardGetTicker       = "GetTicker"
	guardGetBalance      = "GetBalance"
	guardCreateOrder     = "CreateOrder"
	guardCreateStopOrder = "CreateStopOrder"
	guardCancelOrder     = "CancelOrder"
	guardGetOrder        = "GetOrder"
	guardGetOrders       = "GetOrders"
	guardGetOpenOrders   = "GetOpenOrders"
	guardDefault         = "default"
)

func NewGuard() *Guard {
	return &Guard{
		states: make(map[string]*guardState),
		rules: map[string]guardRule{
			// GetTicker runs once per strategy cycle for valuation and signal generation.
			guardGetTicker: {
				maxFailures:     5,
				baseCooldown:    5 * time.Second,
				maximumCooldown: 15 * time.Minute,
			},

			// GetBalance runs in the health check and position sync tasks, less frequent than strategy cycles.
			guardGetBalance: {
				maxFailures:     5,
				baseCooldown:    15 * time.Second,
				maximumCooldown: 15 * time.Minute,
			},

			// GetOrder runs while checking open orders and stop orders, less frequent than health checks.
			guardGetOrder: {
				maxFailures:     3,
				baseCooldown:    30 * time.Second,
				maximumCooldown: 15 * time.Minute,
			},

			// Other operations are normally event-driven: order placement, stop protection,
			// cancellation, and open-order checks happen only when the strategy needs them.
			guardDefault: {
				maxFailures:     2,
				baseCooldown:    time.Minute,
				maximumCooldown: 30 * time.Minute,
			},
		},
	}
}

// Allow checks whether a request may reach the gateway. After a cooldown, one
// request is admitted as a probe; concurrent requests for the same key remain blocked.
func (g *Guard) Allow(method, exchange, symbol string) error {
	g.mu.Lock()
	defer g.mu.Unlock()

	key := guardKey(method, exchange, symbol)
	state, exists := g.states[key]
	if !exists {
		return nil
	}

	if state.blockedUntil.IsZero() {
		return nil
	}

	now := time.Now()
	if now.Before(state.blockedUntil) {
		return &GuardError{
			Key:          key,
			Remaining:    state.blockedUntil.Sub(now),
			PenaltyLevel: state.penaltyLevel,
		}
	}

	// Admit one probe after cooldown expiry. probeActive prevents a burst of
	// concurrent callers from bypassing the cooldown together.
	if state.probeActive {
		return &GuardError{Key: key, PenaltyLevel: state.penaltyLevel, ProbeInProgress: true}
	}

	state.probeActive = true
	return nil
}

// Record updates aggregate state for a request key. Initial failures are tolerated
// up to the operation rule; while a penalty remains, one error reopens the key.
func (g *Guard) Record(method, exchange, symbol string, err error) {
	g.mu.Lock()
	defer g.mu.Unlock()

	key := guardKey(method, exchange, symbol)
	state, exists := g.states[key]
	if !exists {
		if err == nil {
			return
		}
		state = &guardState{}
		g.states[key] = state
	}

	if err == nil {
		// A successful request clears the current failure streak and reduces the penalty
		// level. While a penalty remains, a new failure immediately reopens the cooldown;
		// once the penalty reaches zero, the normal failure threshold applies again.
		state.consecutiveFailures = 0
		state.probeActive = false
		if state.penaltyLevel > 0 {
			state.penaltyLevel -= 2
			if state.penaltyLevel < 0 {
				state.penaltyLevel = 0
			}
		}
		state.blockedUntil = time.Time{}
		if state.penaltyLevel == 0 {
			delete(g.states, key)
		}
		return
	}

	state.probeActive = false
	state.consecutiveFailures++
	rule := g.rule(method)
	// The initial failure streak uses the operation-specific threshold. Once the
	// key has a penalty, every new error reopens it immediately.
	if state.penaltyLevel == 0 && state.consecutiveFailures < rule.maxFailures {
		return
	}

	state.consecutiveFailures = 0
	if state.penaltyLevel < maxPenaltyLevel(rule) {
		state.penaltyLevel++
	}
	state.blockedUntil = time.Now().Add(g.cooldown(rule, state.penaltyLevel))
}

// Release cancels an admitted probe without changing the penalty. It is used
// when the caller context ends locally and no gateway outcome was received.
func (g *Guard) Release(method, exchange, symbol string) {
	g.mu.Lock()
	defer g.mu.Unlock()

	key := guardKey(method, exchange, symbol)
	if state, exists := g.states[key]; exists {
		state.probeActive = false
	}
}

func (g *Guard) rule(method string) guardRule {
	if rule, exists := g.rules[method]; exists {
		return rule
	}
	return g.rules[guardDefault]
}

func (g *Guard) cooldown(rule guardRule, penaltyLevel int) time.Duration {
	cooldown := rule.baseCooldown
	// Penalty levels apply exponential backoff. Jitter avoids synchronized probes,
	// and the rule maximum bounds the resulting delay.
	for level := 1; level < penaltyLevel; level++ {
		if cooldown >= rule.maximumCooldown/2 {
			cooldown = rule.maximumCooldown
			break
		}
		cooldown *= 2
	}

	if cooldown > rule.maximumCooldown {
		cooldown = rule.maximumCooldown
	}
	if cooldown < rule.maximumCooldown {
		jitterLimit := cooldown / 4
		if jitterLimit > 0 {
			cooldown += time.Duration(rand.Int63n(int64(jitterLimit) + 1))
			if cooldown > rule.maximumCooldown {
				cooldown = rule.maximumCooldown
			}
		}
	}
	return cooldown
}

func maxPenaltyLevel(rule guardRule) int {
	level := 1
	cooldown := rule.baseCooldown
	for cooldown < rule.maximumCooldown {
		cooldown *= 2
		level++
	}
	return level
}

func guardKey(method, exchange, symbol string) string {
	if symbol == "" {
		return fmt.Sprintf("%s:%s", method, exchange)
	}
	return fmt.Sprintf("%s:%s:%s", method, exchange, symbol)
}
