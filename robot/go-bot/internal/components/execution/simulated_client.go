package execution

import (
	"context"
	"database/sql"
	"encoding/csv"
	"fmt"
	"log"
	"math"
	"os"
	"strconv"
	"strings"
	"time"

	pb "trading/robot/go-bot/gen/go/v1"
	"trading/robot/go-bot/internal/database/repository"
)

const (
	ExchangeName = "binance" // We need to use a exchange name already migrated in the database
	BudgetAsset  = "USDT"
)

const (
	// Artificial interval in seconds for generating intermediate price ticks (e.g., 300 seconds = 5 minutes)
	artificialInterval = 300
	// Maximum allowed price jump ratio between consecutive ticks (e.g., 2.0 means 200% jump is allowed)
	maxPriceJumpRatio = 2.0
	// Exchange tax rate for calculating fees (e.g., 0.002 = 0.2%)
	exchangeTaxRate = 0.002
)

// Hard-coded strategy configuration for simulation that needs to be updated according to the strategy being tested.
const (
	SimStrategyType    = repository.StrategyMomentumProfit
	SimWindowSeconds   = 43201
	SimMomentumSpec    = "3600:0.05,21600:0.07,43200:0.14"
	SimRequireAll      = false
	SimStopLossPct     = 0.20
	SimProfitTargetPct = 0.15
	SimActivationPct   = 0.00
	SimTrailingStopPct = 0.00
)

// PriceCandle represents a single price tick from the CSV data.
type PriceCandle struct {
	UnixTimestamp int64
	Price         float64
}

// TradePair represents a completed buy-sell cycle for simulator-parity logging.
type TradePair struct {
	EntryTimestamp int64
	EntryDate      string
	EntryPrice     float64
	ExitTimestamp  int64
	ExitDate       string
	ExitPrice      float64
	PnLPct         float64
	ExitReason     string
}

// SimulatedStopOrder represents a simulated stop order in the backtesting environment.
type SimulatedStopOrder struct {
	Id    string
	Price float64
}

// SimulatedClient implements a minimal price server for backtesting.
type SimulatedClient struct {
	initialCapital float64
	assetSymbol    string
	cashOwned      float64
	cryptoOwned    float64
	priceHistory   []PriceCandle
	historyIndex   int
	orderCounter   int
	orders         map[string]*pb.OrderResponse
	tradeCounter   int
	trades         map[string]TradePair
	stopOrder      SimulatedStopOrder
	outputPath     string
}

// NewSimulatedClient creates a new simulated client that loads CSV price data.
func NewSimulatedClient(
	symbol string,
	beginPeriod string,
	endPeriod string,
	inputPath string,
	outputPath string,
	initialUSDT float64,
) (*SimulatedClient, error) {
	if inputPath == "" {
		return nil, fmt.Errorf("input path (CSV price file) is required for simulation")
	}

	sc := &SimulatedClient{
		initialCapital: initialUSDT,
		assetSymbol:    symbol,
		cashOwned:      initialUSDT,
		cryptoOwned:    0.0,
		orderCounter:   1,
		orders:         make(map[string]*pb.OrderResponse),
		tradeCounter:   1,
		trades:         make(map[string]TradePair),
		stopOrder:      SimulatedStopOrder{},
		outputPath:     outputPath,
	}

	if err := sc.loadPriceHistory(inputPath, beginPeriod, endPeriod); err != nil {
		return nil, err
	}

	if len(sc.priceHistory) == 0 {
		return nil, fmt.Errorf(
			"no price data found for symbol %s in the specified period [%s, %s] in %s",
			symbol, beginPeriod, endPeriod, inputPath,
		)
	}

	sc.historyIndex = 0
	return sc, nil
}

// Ping returns a success message.
func (sc *SimulatedClient) Ping(ctx context.Context) (string, error) {
	return "pong", nil
}

// GetTicker returns the current price and advances the simulation.
func (sc *SimulatedClient) GetTicker(
	ctx context.Context, exchange, symbol string,
) (*pb.TickerResponse, error) {
	// End of period reached, close the simulation and write trade log
	if sc.historyIndex >= len(sc.priceHistory) {
		log.Printf("End of price data reached for symbol %s. Writing trade log to %s", symbol, sc.outputPath)
		_ = sc.Close()
		os.Exit(0)
	}

	price := sc.priceHistory[sc.historyIndex].Price
	sc.historyIndex++

	// Check whether the current price has triggered the pending stop order.
	if sc.stopOrder.Id != "" && price <= sc.stopOrder.Price {
		// For the sake of simplicity, cancel the triggered stop order so the robot can place a new one.
		_, _ = sc.CancelOrder(ctx, exchange, symbol, sc.stopOrder.Id)
	}

	return &pb.TickerResponse{
		Symbol: symbol,
		Price:  price,
	}, nil
}

// GetBalance returns current USDT and crypto holdings.
func (sc *SimulatedClient) GetBalance(
	ctx context.Context, exchange, currency string,
) (*pb.BalanceResponse, error) {
	balances := []*pb.BalanceObject{
		{Asset: BudgetAsset, Free: sc.cashOwned, Total: sc.cashOwned},
		{Asset: sc.assetSymbol, Free: sc.cryptoOwned, Total: sc.cryptoOwned},
	}
	return &pb.BalanceResponse{Balances: balances}, nil
}

// CreateOrder executes an order at the same price that triggered it.
func (sc *SimulatedClient) CreateOrder(
	ctx context.Context, req *pb.CreateOrderRequest,
) (*pb.OrderResponse, error) {
	idx := sc.historyIndex - 1
	if idx < 0 {
		return nil, fmt.Errorf("no price seen yet")
	}
	price := sc.priceHistory[idx].Price

	tradeId := fmt.Sprintf("%d", sc.tradeCounter)

	var amount, cost float64
	switch req.Side {
	case repository.OrderSideBuy:
		cost = req.Amount * price
		if sc.cashOwned < cost {
			return nil, fmt.Errorf("insufficient USDT")
		}
		sc.cashOwned -= cost

		amount = req.Amount * (1 - exchangeTaxRate)
		sc.cryptoOwned += amount

		// Record the entry of a new trade pair.
		trade := TradePair{
			EntryTimestamp: sc.priceHistory[idx].UnixTimestamp,
			EntryDate:      time.Unix(sc.priceHistory[idx].UnixTimestamp, 0).UTC().Format("2006-01-02 15:04"),
			EntryPrice:     price,
		}
		sc.trades[tradeId] = trade

	case repository.OrderSideSell:
		amount = req.Amount
		if sc.cryptoOwned < amount {
			return nil, fmt.Errorf("insufficient crypto")
		}
		sc.cryptoOwned -= amount

		cost = amount * price * (1 - exchangeTaxRate)
		sc.cashOwned += cost

		// Record the exit of the trade pair and calculate PnL.
		trade := sc.trades[tradeId]

		trade.ExitTimestamp = sc.priceHistory[idx].UnixTimestamp
		trade.ExitDate = time.Unix(sc.priceHistory[idx].UnixTimestamp, 0).UTC().Format("2006-01-02 15:04")
		trade.ExitPrice = price

		buyPrice := trade.EntryPrice * (1.0 + exchangeTaxRate)
		sellPrice := price * (1.0 - exchangeTaxRate)
		trade.PnLPct = (sellPrice - buyPrice) / buyPrice

		reason := "unknown"
		if trade.PnLPct < 0 {
			reason = "stop_loss"
		} else if SimStrategyType == repository.StrategyMomentumProfit {
			reason = "profit_target"
		} else if SimStrategyType == repository.StrategyMomentumTrailing {
			reason = "trailing_stop"
		}
		trade.ExitReason = reason

		sc.trades[tradeId] = trade
		sc.tradeCounter++

	default:
		return nil, fmt.Errorf("unsupported order side: %s", req.Side)
	}

	orderId := fmt.Sprintf("%d", sc.orderCounter)
	sc.orderCounter++

	resp := &pb.OrderResponse{
		Id:          orderId,
		Symbol:      req.Symbol,
		Side:        req.Side,
		Type:        req.Type,
		Amount:      amount,
		Price:       price,
		Status:      repository.OrderStatusClosed,
		Filled:      amount,
		Cost:        cost,
		Fee:         req.Amount * price * exchangeTaxRate,
		FeeCurrency: BudgetAsset,
		// We adjust to milliseconds to match typical exchange API formats.
		Timestamp: sc.priceHistory[idx].UnixTimestamp * 1000,
	}
	sc.orders[orderId] = resp
	return resp, nil
}

// CreateStopOrder creates a simulated stop order.
func (sc *SimulatedClient) CreateStopOrder(
	ctx context.Context, req *pb.CreateStopOrderRequest,
) (*pb.OrderResponse, error) {
	orderId := fmt.Sprintf("%d", sc.orderCounter)
	sc.orderCounter++

	sc.stopOrder = SimulatedStopOrder{
		Id:    orderId,
		Price: req.StopPrice,
	}

	resp := &pb.OrderResponse{
		Id:          orderId,
		Symbol:      req.Symbol,
		Side:        req.Side,
		Type:        repository.OrderTypeStopMarket,
		Amount:      req.Amount,
		Price:       req.StopPrice,
		Status:      repository.OrderStatusOpen,
		Filled:      req.Amount,
		Cost:        req.Amount * req.StopPrice * (1 - exchangeTaxRate),
		Fee:         req.Amount * req.StopPrice * exchangeTaxRate,
		FeeCurrency: BudgetAsset,
		// We adjust to milliseconds to match typical exchange API formats.
		Timestamp: sc.priceHistory[sc.historyIndex-1].UnixTimestamp * 1000,
	}
	sc.orders[orderId] = resp
	return resp, nil
}

// CancelOrder cancels a simulated order.
func (sc *SimulatedClient) CancelOrder(
	ctx context.Context, exchange, symbol, id string,
) (*pb.CancelOrderResponse, error) {
	if order, ok := sc.orders[id]; ok {
		order.Status = repository.OrderStatusCanceled
	}
	// If the canceled order is the pending stop order, clear it.
	if sc.stopOrder.Id == id {
		sc.stopOrder = SimulatedStopOrder{}
	}
	return &pb.CancelOrderResponse{
		Id:     id,
		Status: repository.OrderStatusCanceled,
	}, nil
}

// GetOrder retrieves details of a simulated order.
func (sc *SimulatedClient) GetOrder(
	ctx context.Context, exchange, symbol, id string,
) (*pb.OrderResponse, error) {
	if order, ok := sc.orders[id]; ok {
		return order, nil
	}
	return nil, fmt.Errorf("order %s not found", id)
}

// GetOpenOrders returns list of open orders (only the stop order in simulation).
func (sc *SimulatedClient) GetOpenOrders(
	ctx context.Context, exchange, symbol string, limit int,
) (*pb.OrdersResponse, error) {
	var openOrders []*pb.OrderResponse
	for _, order := range sc.orders {
		if order.Status == repository.OrderStatusOpen {
			openOrders = append(openOrders, order)
		}
	}
	return &pb.OrdersResponse{Orders: openOrders}, nil
}

// GetRecentTrades returns list of recent trades (empty in simulation).
func (sc *SimulatedClient) GetRecentTrades(
	ctx context.Context, exchange, symbol string, since int64, limit int,
) (*pb.OrdersResponse, error) {
	return &pb.OrdersResponse{Orders: make([]*pb.OrderResponse, 0)}, nil
}

// ResetState resets the simulated client.
func (sc *SimulatedClient) ResetState(ctx context.Context) (*pb.ResetStateResponse, error) {
	sc.cashOwned = sc.initialCapital
	sc.cryptoOwned = 0.0
	sc.historyIndex = 0
	sc.orderCounter = 1
	sc.tradeCounter = 1
	sc.orders = make(map[string]*pb.OrderResponse)
	sc.trades = make(map[string]TradePair)
	sc.stopOrder = SimulatedStopOrder{}
	return &pb.ResetStateResponse{Status: "reset"}, nil
}

// Close writes the trade log to CSV.
func (sc *SimulatedClient) Close() error {
	var file *os.File
	var err error

	if sc.outputPath == "" {
		file = os.Stdout
	} else {
		file, err = os.Create(sc.outputPath)
		if err != nil {
			return err
		}
		defer file.Close()
	}

	writer := csv.NewWriter(file)
	defer writer.Flush()

	if err := writer.Write([]string{
		"entry_timestamp", "entry_date", "entry_price", "exit_timestamp",
		"exit_date", "exit_price", "pnl_pct", "exit_reason",
	}); err != nil {
		return fmt.Errorf("failed to write CSV header: %w", err)
	}

	lastCandle := sc.priceHistory[len(sc.priceHistory)-1]
	tradeId := fmt.Sprintf("%d", sc.tradeCounter)

	// Check if the current trade (last one) is still open
	if trade, ok := sc.trades[tradeId]; ok && trade.ExitReason == "" {
		trade.ExitTimestamp = lastCandle.UnixTimestamp
		trade.ExitDate = time.Unix(lastCandle.UnixTimestamp, 0).UTC().Format("2006-01-02 15:04")
		trade.ExitPrice = lastCandle.Price

		buyPrice := trade.EntryPrice * (1.0 + exchangeTaxRate)
		sellPrice := lastCandle.Price * (1.0 - exchangeTaxRate)
		trade.PnLPct = (sellPrice - buyPrice) / buyPrice
		trade.ExitReason = "end_of_data"

		sc.trades[tradeId] = trade
		sc.tradeCounter++
	}

	// Write all trades to CSV and calculate accumulated PnL
	accumulatedPnL := 1.0
	for i := 1; i < sc.tradeCounter; i++ {
		t := sc.trades[fmt.Sprintf("%d", i)]
		accumulatedPnL *= (1.0 + t.PnLPct)
		if err := writer.Write([]string{
			fmt.Sprintf("%d", t.EntryTimestamp),
			t.EntryDate,
			fmt.Sprintf("%.4f", t.EntryPrice),
			fmt.Sprintf("%d", t.ExitTimestamp),
			t.ExitDate,
			fmt.Sprintf("%.4f", t.ExitPrice),
			fmt.Sprintf("%.4f", t.PnLPct),
			t.ExitReason,
		}); err != nil {
			return fmt.Errorf("failed to write trade to CSV: %w", err)
		}
	}

	// Add end_of_period summary row (Total Portfolio PnL)
	if len(sc.priceHistory) > 0 {
		first := sc.priceHistory[0]
		last := sc.priceHistory[len(sc.priceHistory)-1]

		if err := writer.Write([]string{
			fmt.Sprintf("%d", first.UnixTimestamp),
			time.Unix(first.UnixTimestamp, 0).UTC().Format("2006-01-02 15:04"),
			fmt.Sprintf("%.4f", first.Price),
			fmt.Sprintf("%d", last.UnixTimestamp),
			time.Unix(last.UnixTimestamp, 0).UTC().Format("2006-01-02 15:04"),
			fmt.Sprintf("%.4f", last.Price),
			fmt.Sprintf("%.4f", accumulatedPnL-1.0),
			"end_of_period",
		}); err != nil {
			return fmt.Errorf("failed to write summary row to CSV: %w", err)
		}
	}

	return nil
}

// CurrentTime returns the timestamp of the price just served.
// This is used by the SimulatedClock to advance time in the backtest.
func (sc *SimulatedClient) CurrentTime() time.Time {
	idx := sc.historyIndex - 1
	if idx < 0 {
		idx = 0
	}
	if idx >= len(sc.priceHistory) {
		idx = len(sc.priceHistory) - 1
	}
	return time.Unix(sc.priceHistory[idx].UnixTimestamp, 0).UTC()
}

// GetSimulationStrategies returns a strategy configuration for backtesting.
// This is a simulated version of what should be retrieved from the database in production.
func GetSimulationStrategies(symbol string) ([]repository.StrategyPair, error) {
	var momentumWindows []repository.MomentumWindow
	for _, part := range strings.Split(SimMomentumSpec, ",") {
		var l int
		var t float64
		if _, err := fmt.Sscanf(part, "%d:%f", &l, &t); err == nil {
			momentumWindows = append(momentumWindows, repository.MomentumWindow{
				LookbackSeconds: l,
				Threshold:       t,
			})
		}
	}

	pair := repository.StrategyPair{
		ExchangeName:        ExchangeName,
		InstrumentSymbol:    fmt.Sprintf("%s/%s", symbol, BudgetAsset),
		Type:                SimStrategyType,
		Status:              repository.StrategyEnabled,
		WarmupWindowSeconds: SimWindowSeconds,
		Momentum: repository.StrategyMomentum{
			WindowSeconds: SimWindowSeconds,
			Windows:       momentumWindows,
			RequireAll:    SimRequireAll,
			StopLossPct:   SimStopLossPct,
			ProfitTargetPct: sql.NullFloat64{
				Float64: SimProfitTargetPct,
				Valid:   SimStrategyType == repository.StrategyMomentumProfit,
			},
			ActivationPct: sql.NullFloat64{
				Float64: SimActivationPct,
				Valid:   SimStrategyType == repository.StrategyMomentumTrailing,
			},
			TrailingStopPct: sql.NullFloat64{
				Float64: SimTrailingStopPct,
				Valid:   SimStrategyType == repository.StrategyMomentumTrailing,
			},
		},
	}
	return []repository.StrategyPair{pair}, nil
}

// loadPriceHistory reads CSV file and loads prices with validation and artificial price generation.
func (sc *SimulatedClient) loadPriceHistory(inputFile, begin, end string) error {
	file, err := os.Open(inputFile)
	if err != nil {
		return fmt.Errorf("failed to open price file %s: %w", inputFile, err)
	}
	defer file.Close()

	// Parse begin and end periods (optional)
	var beginUnix, endUnix int64
	if begin != "" {
		t, err := time.Parse("2006-01", begin)
		if err == nil {
			beginUnix = t.Unix()
		}
	}
	if end != "" {
		t, err := time.Parse("2006-01", end)
		if err == nil {
			// Include the whole end month
			endUnix = t.AddDate(0, 1, 0).Unix() - 1
		}
	}

	reader := csv.NewReader(file)
	records, err := reader.ReadAll()
	if err != nil {
		return fmt.Errorf("failed to read CSV file: %w", err)
	}

	if len(records) < 2 {
		return fmt.Errorf("CSV file has insufficient data")
	}

	var previousTimestamp int64
	var previousPrice float64
	firstRow := true
	lineNumber := 1

	for _, record := range records[1:] {
		lineNumber++
		if len(record) < 6 {
			continue
		}

		unixTS, err := strconv.ParseInt(record[0], 10, 64)
		if err != nil {
			continue
		}

		// Filter by period if provided
		if beginUnix > 0 && unixTS < beginUnix {
			continue
		}
		if endUnix > 0 && unixTS > endUnix {
			continue
		}

		price, err := strconv.ParseFloat(record[5], 64)
		if err != nil {
			continue
		}

		if price <= 0.0 {
			continue
		}
		if !firstRow && unixTS < previousTimestamp {
			return fmt.Errorf("price file not sorted at line %d", lineNumber)
		}

		if !firstRow {
			// Skip price jumps that exceed the maximum allowed ratio to avoid unrealistic spikes.
			if math.Abs((price-previousPrice)/previousPrice) > maxPriceJumpRatio {
				continue
			}
		}

		// Generate artificial intermediate price ticks if the gap exceeds the artificial interval.
		if !firstRow && artificialInterval > 0 {
			gap := unixTS - previousTimestamp
			if gap > artificialInterval {
				numSteps := gap / artificialInterval
				priceDiff := price - previousPrice
				priceStep := priceDiff / float64(numSteps)

				for j := int64(1); j < numSteps; j++ {
					t := previousTimestamp + (j * artificialInterval)
					p := previousPrice + (float64(j) * priceStep)
					sc.priceHistory = append(sc.priceHistory, PriceCandle{
						UnixTimestamp: t,
						Price:         p,
					})
				}
			}
		}

		previousTimestamp = unixTS
		previousPrice = price
		firstRow = false

		sc.priceHistory = append(sc.priceHistory, PriceCandle{
			UnixTimestamp: unixTS,
			Price:         price,
		})
	}

	return nil
}
