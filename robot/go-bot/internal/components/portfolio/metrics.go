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

// GetCashBalance returns the current available cash balance.
func (p *Portfolio) GetCashBalance() float64 {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.cashBalance
}

// GetTotalValue calculates the total equity of the portfolio.
func (p *Portfolio) GetTotalValue() float64 {
	p.mu.RLock()
	defer p.mu.RUnlock()

	total := p.cashBalance
	for _, pos := range p.positions {
		total += pos.CurrentPrice * pos.Quantity
	}
	return total
}
