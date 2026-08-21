//go:build unit

package execution

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGuardOpensAfterInitialFailureThreshold(t *testing.T) {
	guard := NewGuard()

	for range 4 {
		require.NoError(t, guard.Allow(guardGetTicker, "binance", "BTC/USDT"))
		guard.Record(guardGetTicker, "binance", "BTC/USDT", errors.New("gateway failure"))
	}

	require.NoError(t, guard.Allow(guardGetTicker, "binance", "BTC/USDT"))
	guard.Record(guardGetTicker, "binance", "BTC/USDT", errors.New("gateway failure"))

	err := guard.Allow(guardGetTicker, "binance", "BTC/USDT")
	var guardErr *GuardError
	require.ErrorAs(t, err, &guardErr)
	assert.Equal(t, 1, guardErr.PenaltyLevel)
}

func TestGuardErrorMessages(t *testing.T) {
	t.Run("cooldown", func(t *testing.T) {
		err := &GuardError{
			Key:          "GetTicker:binance:BTC/USDT",
			Remaining:    1500 * time.Millisecond,
			PenaltyLevel: 2,
		}

		assert.Equal(
			t,
			"execution guard blocked GetTicker:binance:BTC/USDT: cooldown remaining 1.5s, penalty level 2",
			err.Error(),
		)
	})

	t.Run("probe in progress", func(t *testing.T) {
		err := &GuardError{
			Key:             "GetTicker:binance:BTC/USDT",
			PenaltyLevel:    2,
			ProbeInProgress: true,
		}

		assert.Equal(
			t,
			"execution guard blocked GetTicker:binance:BTC/USDT: recovery probe in progress",
			err.Error(),
		)
	})
}

func TestGuardRecoveryFailureReopensImmediately(t *testing.T) {
	guard := NewGuard()
	method := guardGetTicker
	exchange := "binance"
	symbol := "BTC/USDT"

	openGuardKey(t, guard, method, exchange, symbol)
	state := guard.states[guardKey(method, exchange, symbol)]
	state.blockedUntil = time.Now().Add(-time.Second)
	require.NoError(t, guard.Allow(method, exchange, symbol))
	guard.Record(method, exchange, symbol, errors.New("probe failure"))

	state = guard.states[guardKey(method, exchange, symbol)]
	assert.Equal(t, 2, state.penaltyLevel)
	assert.False(t, state.blockedUntil.IsZero())
}

func TestGuardSuccessfulRecoveryReducesPenaltyByTwo(t *testing.T) {
	guard := NewGuard()
	method := guardGetTicker
	exchange := "binance"
	symbol := "BTC/USDT"

	openGuardKey(t, guard, method, exchange, symbol)
	state := guard.states[guardKey(method, exchange, symbol)]
	state.penaltyLevel = 3
	state.blockedUntil = time.Now().Add(-time.Second)
	require.NoError(t, guard.Allow(method, exchange, symbol))
	guard.Record(method, exchange, symbol, nil)

	state = guard.states[guardKey(method, exchange, symbol)]
	require.NotNil(t, state)
	assert.Equal(t, 1, state.penaltyLevel)

	guard.Record(method, exchange, symbol, nil)
	_, exists := guard.states[guardKey(method, exchange, symbol)]
	assert.False(t, exists)
}

func TestGuardCapsPenaltyAtMaximumLevel(t *testing.T) {
	guard := NewGuard()
	method := guardGetTicker
	exchange := "binance"
	symbol := "BTC/USDT"
	key := guardKey(method, exchange, symbol)

	openGuardKey(t, guard, method, exchange, symbol)
	rule := guard.rule(method)
	maximumPenalty := maxPenaltyLevel(rule)

	for guard.states[key].penaltyLevel < maximumPenalty {
		guard.states[key].blockedUntil = time.Now().Add(-time.Second)
		require.NoError(t, guard.Allow(method, exchange, symbol))
		guard.Record(method, exchange, symbol, errors.New("probe failure"))
	}

	assert.Equal(t, maximumPenalty, guard.states[key].penaltyLevel)
	guard.states[key].blockedUntil = time.Now().Add(-time.Second)
	require.NoError(t, guard.Allow(method, exchange, symbol))
	guard.Record(method, exchange, symbol, errors.New("probe failure"))
	assert.Equal(t, maximumPenalty, guard.states[key].penaltyLevel)
}

func TestGuardReleaseClearsProbeWithoutChangingPenalty(t *testing.T) {
	guard := NewGuard()
	method := guardGetTicker
	exchange := "binance"
	symbol := "BTC/USDT"

	openGuardKey(t, guard, method, exchange, symbol)
	state := guard.states[guardKey(method, exchange, symbol)]
	state.blockedUntil = time.Now().Add(-time.Second)
	require.NoError(t, guard.Allow(method, exchange, symbol))

	guard.Release(method, exchange, symbol)

	state = guard.states[guardKey(method, exchange, symbol)]
	assert.Equal(t, 1, state.penaltyLevel)
	assert.False(t, state.probeActive)
	require.NoError(t, guard.Allow(method, exchange, symbol))
}

func TestGuardGetTickerLongRecoveryScenario(t *testing.T) {
	guard := NewGuard()
	method := guardGetTicker
	exchange := "binance"
	symbol := "BTC/USDT"
	key := guardKey(method, exchange, symbol)

	// The initial five errors open the circuit at penalty level one.
	for attempt := 1; attempt <= 5; attempt++ {
		require.NoError(t, guard.Allow(method, exchange, symbol))
		guard.Record(method, exchange, symbol, errors.New("gateway failure"))
		if attempt < 5 {
			require.NoError(t, guard.Allow(method, exchange, symbol))
		}
	}
	assert.Equal(t, 1, guard.states[key].penaltyLevel)

	// Failed probes increase the penalty exponentially until GetTicker reaches
	// its maximum penalty, after which failures keep the cap unchanged.
	for expectedPenalty := 2; expectedPenalty <= maxPenaltyLevel(guard.rule(method)); expectedPenalty++ {
		admitExpiredProbe(t, guard, method, exchange, symbol)
		guard.Record(method, exchange, symbol, errors.New("gateway failure"))
		assert.Equal(t, expectedPenalty, guard.states[key].penaltyLevel)
		assertBlocked(t, guard, method, exchange, symbol, expectedPenalty)
	}

	for range 2 {
		admitExpiredProbe(t, guard, method, exchange, symbol)
		guard.Record(method, exchange, symbol, errors.New("gateway failure"))
		assert.Equal(t, maxPenaltyLevel(guard.rule(method)), guard.states[key].penaltyLevel)
	}

	// Two successful probes reduce the level by two each time, but do not yet
	// remove the state from the guard.
	for _, expectedPenalty := range []int{7, 5} {
		admitExpiredProbe(t, guard, method, exchange, symbol)
		guard.Record(method, exchange, symbol, nil)
		assert.Equal(t, expectedPenalty, guard.states[key].penaltyLevel)
	}

	// While a penalty remains, each error immediately reopens the circuit.
	for _, expectedPenalty := range []int{6, 7, 8} {
		admitExpiredProbe(t, guard, method, exchange, symbol)
		guard.Record(method, exchange, symbol, errors.New("gateway failure"))
		assert.Equal(t, expectedPenalty, guard.states[key].penaltyLevel)
	}

	// Four of the six successes restore the penalty to zero and remove the state;
	// the remaining two are ordinary untracked successful requests.
	for success := 1; success <= 6; success++ {
		if _, exists := guard.states[key]; exists {
			admitExpiredProbe(t, guard, method, exchange, symbol)
		} else {
			require.NoError(t, guard.Allow(method, exchange, symbol))
		}
		guard.Record(method, exchange, symbol, nil)
		if success <= 3 {
			assert.Equal(t, 8-2*success, guard.states[key].penaltyLevel)
		} else {
			_, exists := guard.states[key]
			assert.False(t, exists)
		}
	}

	// After recovery, two errors only start a fresh initial failure streak.
	for attempt := 1; attempt <= 2; attempt++ {
		require.NoError(t, guard.Allow(method, exchange, symbol))
		guard.Record(method, exchange, symbol, errors.New("gateway failure"))
		state := guard.states[key]
		require.NotNil(t, state)
		assert.Equal(t, attempt, state.consecutiveFailures)
		assert.Zero(t, state.penaltyLevel)
		assert.True(t, state.blockedUntil.IsZero())
	}

	// State for another symbol is independent and remains untouched.
	otherSymbol := "ETH/USDT"
	require.NoError(t, guard.Allow(method, exchange, otherSymbol))
	_, exists := guard.states[guardKey(method, exchange, otherSymbol)]
	assert.False(t, exists)
}

func admitExpiredProbe(t *testing.T, guard *Guard, method, exchange, symbol string) {
	t.Helper()
	key := guardKey(method, exchange, symbol)
	state, exists := guard.states[key]
	require.True(t, exists)
	state.blockedUntil = time.Now().Add(-time.Second)
	require.NoError(t, guard.Allow(method, exchange, symbol))
}

func assertBlocked(t *testing.T, guard *Guard, method, exchange, symbol string, penalty int) {
	t.Helper()
	err := guard.Allow(method, exchange, symbol)
	var guardErr *GuardError
	require.ErrorAs(t, err, &guardErr)
	assert.Equal(t, penalty, guardErr.PenaltyLevel)
}

func openGuardKey(t *testing.T, guard *Guard, method, exchange, symbol string) {
	t.Helper()
	for range guard.rule(method).maxFailures {
		require.NoError(t, guard.Allow(method, exchange, symbol))
		guard.Record(method, exchange, symbol, errors.New("gateway failure"))
	}
}
