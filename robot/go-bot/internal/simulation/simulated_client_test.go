//go:build unit

package simulation

import (
	"context"
	"fmt"
	"os"
	"testing"

	pb "trading/robot/go-bot/gen/go/v1"
	"trading/robot/go-bot/internal/database/repository"

	"github.com/stretchr/testify/assert"
)

func TestNewSimulatedClient(t *testing.T) {
	tests := []struct {
		name        string
		symbol      string
		begin       string
		end         string
		inputPath   string
		expectError bool
		errorMsg    string
		check       func(*testing.T, *SimulatedClient)
	}{
		{
			name:      "successful load with artificial ticks",
			symbol:    "BTC",
			begin:     "2023-11",
			end:       "2023-11",
			inputPath: "testdata/prices_valid.csv",
			check: func(t *testing.T, sc *SimulatedClient) {
				assert.Equal(t, 5, len(sc.priceHistory))
				assert.Equal(t, 10.0, sc.priceHistory[0].Price)
				assert.Equal(t, 10.5, sc.priceHistory[1].Price)
				assert.Equal(t, 11.0, sc.priceHistory[2].Price)
				assert.Equal(t, 11.5, sc.priceHistory[3].Price)
				assert.Equal(t, 12.0, sc.priceHistory[4].Price)
			},
		},
		{
			name:        "unsorted price file",
			symbol:      "BTC",
			begin:       "1970-01",
			end:         "2030-01",
			inputPath:   "testdata/prices_unsorted.csv",
			expectError: true,
			errorMsg:    "price file not sorted",
		},
		{
			name:        "missing periods",
			symbol:      "BTC",
			begin:       "",
			end:         "",
			inputPath:   "testdata/prices_valid.csv",
			expectError: true,
			errorMsg:    "begin and end periods are mandatory",
		},
		{
			name:        "invalid begin format",
			symbol:      "BTC",
			begin:       "2024-01-01",
			end:         "2024-02",
			inputPath:   "testdata/prices_valid.csv",
			expectError: true,
			errorMsg:    "invalid begin period format",
		},
		{
			name:        "invalid end format",
			symbol:      "BTC",
			begin:       "2024-01",
			end:         "2024-02-01",
			inputPath:   "testdata/prices_valid.csv",
			expectError: true,
			errorMsg:    "invalid end period format",
		},
		{
			name:        "empty file header only",
			symbol:      "BTC",
			begin:       "1970-01",
			end:         "2030-01",
			inputPath:   "testdata/prices_empty.csv",
			expectError: true,
			errorMsg:    "no price data found ",
		},
		{
			name:        "empty input path",
			symbol:      "BTC",
			begin:       "1970-01",
			end:         "2030-01",
			expectError: true,
			errorMsg:    "input path (CSV price file) is required",
		},
		{
			name:        "successful success with inconsistent file",
			symbol:      "BTC",
			begin:       "2023-11",
			end:         "2023-11",
			inputPath:   "testdata/prices_inconsistent.csv",
			expectError: false,
			check: func(t *testing.T, sc *SimulatedClient) {
				assert.Equal(t, 5, len(sc.priceHistory))
				assert.Equal(t, 12.0, sc.priceHistory[0].Price)
				assert.Equal(t, 11.5, sc.priceHistory[1].Price)
				assert.Equal(t, 11.0, sc.priceHistory[2].Price)
				assert.Equal(t, 10.5, sc.priceHistory[3].Price)
				assert.Equal(t, 10.0, sc.priceHistory[4].Price)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sc, err := NewSimulatedClient(tt.symbol, tt.begin, tt.end, tt.inputPath, "", 1000.0)
			if tt.expectError {
				assert.Error(t, err)
				if tt.errorMsg != "" {
					assert.Contains(t, err.Error(), tt.errorMsg)
				}
			} else {
				if !assert.NoError(t, err) {
					return
				}
				if tt.check != nil {
					tt.check(t, sc)
				}
			}
		})
	}
}

func TestSimulatedClient_Ping(t *testing.T) {
	sc := &SimulatedClient{}
	ctx := context.Background()

	message, err := sc.Ping(ctx)
	assert.NoError(t, err)
	assert.Equal(t, "pong", message)
}

func TestSimulatedClient_GetTicker(t *testing.T) {
	sc := &SimulatedClient{
		priceHistory: []PriceCandle{
			{UnixTimestamp: 1000, Price: 100.0},
			{UnixTimestamp: 1100, Price: 90.0},
		},
		orders: make(map[string]*pb.OrderResponse),
	}

	ctx := context.Background()

	t.Run("ticker advancement", func(t *testing.T) {
		resp, err := sc.GetTicker(ctx, "binance", "BTC/USDT")
		assert.NoError(t, err)
		assert.Equal(t, 100.0, resp.Price)
		assert.Equal(t, 1, sc.historyIndex)
	})

	t.Run("stop order trigger", func(t *testing.T) {
		sc.stopOrder = SimulatedStopOrder{Id: "stop-1", Price: 95.0}
		sc.orders["stop-1"] = &pb.OrderResponse{Id: "stop-1", Status: repository.OrderStatusOpen}

		// Next price is 90.0, which is below 95.0
		resp, err := sc.GetTicker(ctx, "binance", "BTC/USDT")
		assert.NoError(t, err)
		assert.Equal(t, 90.0, resp.Price)

		// Stop order should be cleared (canceled)
		assert.Empty(t, sc.stopOrder.Id)
		assert.Equal(t, repository.OrderStatusCanceled, sc.orders["stop-1"].Status)
	})
}

func TestSimulatedClient_GetBalance(t *testing.T) {
	sc := &SimulatedClient{
		initialCapital: 1000.0,
		cashOwned:      500.0,
		cryptoOwned:    2.0,
		assetSymbol:    "BTC",
	}
	ctx := context.Background()

	resp, err := sc.GetBalance(ctx, "binance", "USDT")
	assert.NoError(t, err)
	assert.Len(t, resp.Balances, 2)
	assert.Equal(t, "USDT", resp.Balances[0].Asset)
	assert.Equal(t, 500.0, resp.Balances[0].Free)
	assert.Equal(t, "BTC", resp.Balances[1].Asset)
	assert.Equal(t, 2.0, resp.Balances[1].Free)
}

func TestSimulatedClient_CreateOrder(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name        string
		setupClient func() *SimulatedClient
		req         *pb.CreateOrderRequest
		expectError bool
		errorMsg    string
		checkResult func(*testing.T, *SimulatedClient, error)
	}{
		{
			name: "successful buy",
			setupClient: func() *SimulatedClient {
				return &SimulatedClient{
					assetSymbol:  "BTC",
					cashOwned:    1000.0,
					cryptoOwned:  0.0,
					priceHistory: []PriceCandle{{UnixTimestamp: 1000, Price: 100.0}},
					historyIndex: 1,
					orders:       make(map[string]*pb.OrderResponse),
					trades:       make(map[string]TradePair),
					tradeCounter: 1,
					orderCounter: 1,
				}
			},
			req: &pb.CreateOrderRequest{Side: repository.OrderSideBuy, Amount: 5.0, Symbol: "BTC/USDT"},
			checkResult: func(t *testing.T, sc *SimulatedClient, err error) {
				assert.NoError(t, err)
				assert.Equal(t, 500.0, sc.cashOwned)
				assert.InDelta(t, 4.99, sc.cryptoOwned, 0.0001)
			},
		},
		{
			name: "insufficient funds buy",
			setupClient: func() *SimulatedClient {
				return &SimulatedClient{
					assetSymbol:  "BTC",
					cashOwned:    100.0,
					cryptoOwned:  0.0,
					priceHistory: []PriceCandle{{UnixTimestamp: 1000, Price: 100.0}},
					historyIndex: 1,
					orders:       make(map[string]*pb.OrderResponse),
					trades:       make(map[string]TradePair),
					tradeCounter: 1,
					orderCounter: 1,
				}
			},
			req:         &pb.CreateOrderRequest{Side: repository.OrderSideBuy, Amount: 10.0, Symbol: "BTC/USDT"},
			expectError: true,
			errorMsg:    "insufficient USDT",
		},
		{
			name: "successful sell at profit",
			setupClient: func() *SimulatedClient {
				return &SimulatedClient{
					assetSymbol: "BTC",
					cashOwned:   500.0,
					cryptoOwned: 4.99,
					priceHistory: []PriceCandle{
						{UnixTimestamp: 1000, Price: 100.0},
						{UnixTimestamp: 2000, Price: 110.0},
					},
					historyIndex: 2,
					orders:       make(map[string]*pb.OrderResponse),
					trades: map[string]TradePair{
						"1": {EntryTimestamp: 1000, EntryPrice: 100.0},
					},
					tradeCounter: 1,
					orderCounter: 2,
				}
			},
			req: &pb.CreateOrderRequest{Side: repository.OrderSideSell, Amount: 4.99, Symbol: "BTC/USDT"},
			checkResult: func(t *testing.T, sc *SimulatedClient, err error) {
				assert.NoError(t, err)
				assert.InDelta(t, 1047.80, sc.cashOwned, 0.01)
				assert.Equal(t, 0.0, sc.cryptoOwned)
				assert.Equal(t, 2, sc.tradeCounter)
			},
		},
		{
			name: "successful sell at loss",
			setupClient: func() *SimulatedClient {
				return &SimulatedClient{
					assetSymbol: "BTC",
					cashOwned:   500.0,
					cryptoOwned: 4.99,
					priceHistory: []PriceCandle{
						{UnixTimestamp: 1000, Price: 100.0},
						{UnixTimestamp: 2000, Price: 90.0},
					},
					historyIndex: 2,
					orders:       make(map[string]*pb.OrderResponse),
					trades: map[string]TradePair{
						"1": {EntryTimestamp: 1000, EntryPrice: 100.0},
					},
					tradeCounter: 1,
					orderCounter: 2,
				}
			},
			req: &pb.CreateOrderRequest{Side: repository.OrderSideSell, Amount: 4.99, Symbol: "BTC/USDT"},
			checkResult: func(t *testing.T, sc *SimulatedClient, err error) {
				assert.NoError(t, err)
				assert.InDelta(t, 948.20, sc.cashOwned, 0.01)
				assert.Equal(t, 0.0, sc.cryptoOwned)
			},
		},
		{
			name: "insufficient crypto sell",
			setupClient: func() *SimulatedClient {
				return &SimulatedClient{
					assetSymbol:  "BTC",
					cashOwned:    1000.0,
					cryptoOwned:  0.0,
					priceHistory: []PriceCandle{{UnixTimestamp: 1000, Price: 100.0}},
					historyIndex: 1,
					orders:       make(map[string]*pb.OrderResponse),
					trades:       make(map[string]TradePair),
					tradeCounter: 1,
					orderCounter: 1,
				}
			},
			req:         &pb.CreateOrderRequest{Side: repository.OrderSideSell, Amount: 1.0, Symbol: "BTC/USDT"},
			expectError: true,
			errorMsg:    "insufficient crypto",
		},
		{
			name: "invalid side",
			setupClient: func() *SimulatedClient {
				return &SimulatedClient{
					assetSymbol:  "BTC",
					cashOwned:    1000.0,
					cryptoOwned:  0.0,
					priceHistory: []PriceCandle{{UnixTimestamp: 1000, Price: 100.0}},
					historyIndex: 1,
					orders:       make(map[string]*pb.OrderResponse),
					trades:       make(map[string]TradePair),
					tradeCounter: 1,
					orderCounter: 1,
				}
			},
			req:         &pb.CreateOrderRequest{Side: "invalid", Amount: 1.0, Symbol: "BTC/USDT"},
			expectError: true,
			errorMsg:    "unsupported order side",
		},
		{
			name: "simulation not started",
			setupClient: func() *SimulatedClient {
				return &SimulatedClient{
					assetSymbol:  "BTC",
					cashOwned:    1000.0,
					cryptoOwned:  0.0,
					priceHistory: []PriceCandle{{UnixTimestamp: 1000, Price: 100.0}},
					historyIndex: 0,
					orders:       make(map[string]*pb.OrderResponse),
					trades:       make(map[string]TradePair),
					tradeCounter: 1,
					orderCounter: 1,
				}
			},
			req:         &pb.CreateOrderRequest{Side: repository.OrderSideBuy, Amount: 1.0, Symbol: "BTC/USDT"},
			expectError: true,
			errorMsg:    "no price seen yet",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sc := tt.setupClient()
			_, err := sc.CreateOrder(ctx, tt.req)
			if tt.expectError {
				assert.Error(t, err)
				if tt.errorMsg != "" {
					assert.Contains(t, err.Error(), tt.errorMsg)
				}
			} else {
				if tt.checkResult != nil {
					tt.checkResult(t, sc, err)
				}
			}
		})
	}
}

func TestSimulatedClient_CreateStopOrder(t *testing.T) {
	sc := &SimulatedClient{
		priceHistory: []PriceCandle{{UnixTimestamp: 1000, Price: 100.0}},
		historyIndex: 1,
		orders:       make(map[string]*pb.OrderResponse),
	}
	ctx := context.Background()

	resp, err := sc.CreateStopOrder(ctx, &pb.CreateStopOrderRequest{StopPrice: 50.0, Amount: 1.0})
	assert.NoError(t, err)
	assert.Equal(t, 50.0, sc.stopOrder.Price)
	assert.Equal(t, repository.OrderStatusOpen, resp.Status)
}

func TestSimulatedClient_CancelOrder(t *testing.T) {
	sc := &SimulatedClient{
		orders: map[string]*pb.OrderResponse{
			"test":  {Id: "test", Status: repository.OrderStatusOpen},
			"test2": {Id: "test2", Status: repository.OrderStatusOpen},
		},
		stopOrder: SimulatedStopOrder{Id: "test2"},
	}

	ctx := context.Background()

	t.Run("cancel order", func(t *testing.T) {
		resp, err := sc.CancelOrder(ctx, "binance", "BTC/USDT", "test")
		assert.NoError(t, err)
		assert.Equal(t, repository.OrderStatusCanceled, resp.Status)
		assert.NotEmpty(t, sc.stopOrder.Id)
	})

	t.Run("cancel stoporder", func(t *testing.T) {
		resp, err := sc.CancelOrder(ctx, "binance", "BTC/USDT", "test2")
		assert.NoError(t, err)
		assert.Equal(t, repository.OrderStatusCanceled, resp.Status)
		assert.Empty(t, sc.stopOrder.Id)
	})
}

func TestSimulatedClient_GetOrdersAndTrades(t *testing.T) {
	sc := &SimulatedClient{
		orders: map[string]*pb.OrderResponse{
			"1": {Id: "1", Status: repository.OrderStatusOpen},
			"2": {Id: "2", Status: repository.OrderStatusClosed},
		},
	}

	ctx := context.Background()

	t.Run("get order", func(t *testing.T) {
		resp, err := sc.GetOrder(ctx, "binance", "BTC/USDT", "2")
		assert.NoError(t, err)
		assert.Equal(t, "2", resp.Id)
		assert.Equal(t, repository.OrderStatusClosed, resp.Status)
	})

	t.Run("get order not found", func(t *testing.T) {
		resp, err := sc.GetOrder(ctx, "binance", "BTC/USDT", "invalid")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "not found")
		assert.Nil(t, resp)
	})

	t.Run("get open orders", func(t *testing.T) {
		resp, err := sc.GetOpenOrders(ctx, "binance", "BTC/USDT", 0)
		assert.NoError(t, err)
		assert.Len(t, resp.Orders, 1)
		assert.Equal(t, "1", resp.Orders[0].Id)
	})

	t.Run("get recent trades", func(t *testing.T) {
		resp, err := sc.GetRecentTrades(ctx, "binance", "BTC/USDT", 0, 0)
		assert.NoError(t, err)
		assert.Empty(t, resp.Orders)
	})
}

func TestSimulatedClient_ResetState(t *testing.T) {
	sc := &SimulatedClient{
		initialCapital: 1000.0,
		cashOwned:      500.0,
		cryptoOwned:    1.0,
		historyIndex:   10,
		orders:         map[string]*pb.OrderResponse{"1": {}},
		trades:         map[string]TradePair{"1": {}},
		tradeCounter:   5,
		orderCounter:   5,
		stopOrder:      SimulatedStopOrder{Id: "stop"},
	}

	sc.ResetState(context.Background())

	assert.Equal(t, 1000.0, sc.cashOwned)
	assert.Equal(t, 0.0, sc.cryptoOwned)
	assert.Equal(t, 0, sc.historyIndex)
	assert.Empty(t, sc.orders)
	assert.Empty(t, sc.trades)
	assert.Equal(t, 1, sc.tradeCounter)
	assert.Equal(t, 1, sc.orderCounter)
	assert.Empty(t, sc.stopOrder.Id)
}

func TestSimulatedClient_Close(t *testing.T) {
	t.Run("writes to file and close trade", func(t *testing.T) {
		outputPath := "testdata/trades_output.csv"
		defer os.Remove(outputPath)

		sc := &SimulatedClient{
			outputPath:   outputPath,
			priceHistory: []PriceCandle{{UnixTimestamp: 1000, Price: 100.0}, {UnixTimestamp: 2000, Price: 100.0}},
			trades: map[string]TradePair{
				"1": {EntryPrice: 100.0, EntryTimestamp: 1000, ExitReason: ""}, // Still open
			},
			tradeCounter: 1,
		}

		err := sc.Close()
		assert.NoError(t, err)

		_, err = os.Stat(outputPath)
		assert.NoError(t, err)
	})

	t.Run("writes to stdout", func(t *testing.T) {
		sc := &SimulatedClient{
			outputPath:   "",
			priceHistory: []PriceCandle{{UnixTimestamp: 1000, Price: 100.0}, {UnixTimestamp: 2000, Price: 100.0}},
		}

		err := sc.Close()
		assert.NoError(t, err)
	})

	t.Run("fails to create file", func(t *testing.T) {
		sc := &SimulatedClient{
			outputPath: "/invalid/path/trades_output.csv",
		}

		err := sc.Close()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to create output file")
	})
}

func TestSimulatedClient_CurrentTime(t *testing.T) {
	sc := &SimulatedClient{
		priceHistory: []PriceCandle{{UnixTimestamp: 1000}, {UnixTimestamp: 2000}},
		historyIndex: 1,
	}

	assert.Equal(t, int64(1000), sc.CurrentTime().Unix())
	sc.historyIndex = 5 // out of bounds
	assert.Equal(t, int64(2000), sc.CurrentTime().Unix())
}

func TestSimulatedClient_GetSimulationStrategies(t *testing.T) {
	symbol := "SOL"
	res, err := GetSimulationStrategies(symbol)
	assert.NoError(t, err)
	assert.Equal(t, res[0].ExchangeName, ExchangeName)
	assert.Equal(t, res[0].InstrumentSymbol, fmt.Sprintf("%s/%s", symbol, BudgetAsset))
	assert.Equal(t, res[0].Type, SimStrategyType)
	assert.Equal(t, res[0].Status, repository.StrategyEnabled)
	assert.Equal(t, res[0].WarmupWindowSeconds, SimWindowSeconds)
	assert.Equal(t, res[0].Momentum.WindowSeconds, SimWindowSeconds)
	assert.Equal(t, res[0].Momentum.RequireAll, SimRequireAll)
	assert.Equal(t, res[0].Momentum.StopLossPct, SimStopLossPct)
	assert.Equal(t, res[0].Momentum.ProfitTargetPct.Float64, SimProfitTargetPct)
	assert.Equal(t, res[0].Momentum.ActivationPct.Float64, SimActivationPct)
	assert.Equal(t, res[0].Momentum.TrailingStopPct.Float64, SimTrailingStopPct)

}
