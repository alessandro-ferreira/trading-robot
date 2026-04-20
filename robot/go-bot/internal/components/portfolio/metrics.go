package portfolio

// GetOpenPositionsCount returns the number of currently active positions.
func (p *Portfolio) GetOpenPositionsCount() int {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return len(p.positions)
}

// GetPosition returns a copy of the position for a given symbol.
func (p *Portfolio) GetPosition(exchange, symbol string) (*Position, bool) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	key := makeKey(exchange, symbol)
	pos, exists := p.positions[key]
	if !exists {
		return nil, false
	}
	posCopy := *pos
	return &posCopy, true
}

// GetCashBalance returns the available cash balance for a specific exchange and asset.
func (p *Portfolio) GetCashBalance(exchange, asset string) float64 {
	p.mu.RLock()
	defer p.mu.RUnlock()
	key := makeKey(exchange, asset)
	if bal, ok := p.cashBalances[key]; ok {
		return bal.Free
	}
	return 0
}

// GetTotalValue calculates the total equity of the portfolio grouped by quote currency.
func (p *Portfolio) GetTotalValue() map[string]float64 {
	p.mu.RLock()
	defer p.mu.RUnlock()

	totals := make(map[string]float64)
	for _, bal := range p.cashBalances {
		totals[bal.Asset] += bal.Free
	}
	for _, pos := range p.positions {
		_, quote := splitSymbol(pos.Symbol)
		if quote != "" {
			totals[quote] += pos.CurrentPrice * pos.Quantity
		}
	}
	return totals
}
