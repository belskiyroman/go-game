package rules

import "math"

// Region is the empty area around point i, with the dead stones counted as empty, and its owner:
// the only color that borders it, or Empty when both colors (or none) do. Returns nil for a live stone.
func Region(b *Board, dead map[int]bool, i int) (points []int, owner Color) {
	occ := func(p int) Color {
		if dead[p] {
			return Empty
		}
		return b.Points[p]
	}
	if occ(i) != Empty {
		return nil, Empty
	}
	borders := map[Color]bool{}
	seen := map[int]bool{i: true}
	stack := []int{i}
	for len(stack) > 0 {
		p := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		points = append(points, p)
		for _, q := range b.Neighbors(p) {
			if c := occ(q); c != Empty {
				borders[c] = true
			} else if !seen[q] {
				seen[q] = true
				stack = append(stack, q)
			}
		}
	}
	if len(borders) == 1 {
		for c := range borders {
			owner = c
		}
	}
	return points, owner
}

// Territory finds empty regions surrounded by only one color, ignoring the dead stones
// (their points count as empty). Points the players marked neutral are no one's territory.
// Returns the points owned by black and by white.
func Territory(b *Board, dead, neutral map[int]bool) (black, white []int) {
	seen := make([]bool, len(b.Points))
	for i := range b.Points {
		if seen[i] {
			continue
		}
		region, owner := Region(b, dead, i)
		for _, p := range region {
			seen[p] = true
		}
		for _, p := range region {
			if neutral[p] {
				continue
			}
			switch owner {
			case Black:
				black = append(black, p)
			case White:
				white = append(white, p)
			}
		}
	}
	return black, white
}

// Score is the final count under territory scoring: territory + prisoners (+ komi for white).
type Score struct {
	BlackTerritory int     `json:"bTerritory"`
	WhiteTerritory int     `json:"wTerritory"`
	BlackPrisoners int     `json:"bPrisoners"` // stones black captured during play + dead white stones
	WhitePrisoners int     `json:"wPrisoners"`
	Komi           float64 `json:"komi"`
	Black          float64 `json:"b"`
	White          float64 `json:"w"`
}

// Count computes the score. capsB/capsW are stones captured during play by black/white.
func Count(b *Board, dead, neutral map[int]bool, capsB, capsW int, komi float64) Score {
	tb, tw := Territory(b, dead, neutral)
	deadB, deadW := 0, 0
	for i := range dead {
		switch b.Points[i] {
		case Black:
			deadB++
		case White:
			deadW++
		}
	}
	s := Score{
		BlackTerritory: len(tb), WhiteTerritory: len(tw),
		BlackPrisoners: capsB + deadW, WhitePrisoners: capsW + deadB,
		Komi: komi,
	}
	s.Black = float64(s.BlackTerritory + s.BlackPrisoners)
	s.White = float64(s.WhiteTerritory+s.WhitePrisoners) + komi
	return s
}

// EstimateDead guesses which groups are dead at the end of the game. It is a heuristic
// (an influence map plus eye counting); players confirm it or choose to play on.
func EstimateDead(b *Board) map[int]bool {
	n := b.Size
	infl := make([]float64, len(b.Points))
	// influence: black positive, white negative, decaying with distance
	radius := 4
	for i, c := range b.Points {
		if c == Empty {
			continue
		}
		sign := 1.0
		if c == White {
			sign = -1.0
		}
		x0, y0 := b.XY(i)
		for dy := -radius; dy <= radius; dy++ {
			for dx := -radius; dx <= radius; dx++ {
				x, y := x0+dx, y0+dy
				if x < 0 || y < 0 || x >= n || y >= n {
					continue
				}
				d2 := float64(dx*dx + dy*dy)
				infl[b.Index(x, y)] += sign * math.Exp(-d2/4.0)
			}
		}
	}
	dead := map[int]bool{}
	seen := map[int]bool{}
	for i, c := range b.Points {
		if c == Empty || seen[i] {
			continue
		}
		stones, libs := b.Group(i)
		for _, s := range stones {
			seen[s] = true
		}
		// sum influence over the group and the empty points around it
		area := map[int]bool{}
		for _, s := range stones {
			area[s] = true
			for _, q := range b.Neighbors(s) {
				if b.Points[q] == Empty {
					area[q] = true
					for _, r := range b.Neighbors(q) {
						if b.Points[r] == Empty {
							area[r] = true
						}
					}
				}
			}
		}
		sum := 0.0
		for p := range area {
			sum += infl[p]
		}
		avg := sum / float64(len(area))
		own := 1.0
		if c == White {
			own = -1.0
		}
		eyes := countEyes(b, stones, c)
		// a group is dead when the area around it clearly belongs to the opponent
		// and it has no room for two eyes
		if avg*own < -0.35 && eyes < 2 && libs <= 6 {
			for _, s := range stones {
				dead[s] = true
			}
		}
	}
	return dead
}

// countEyes counts small empty regions (up to 3 points) bordered only by color c and touching the group.
func countEyes(b *Board, group []int, c Color) int {
	inGroup := map[int]bool{}
	for _, s := range group {
		inGroup[s] = true
	}
	seen := map[int]bool{}
	eyes := 0
	for _, s := range group {
		for _, q := range b.Neighbors(s) {
			if b.Points[q] != Empty || seen[q] {
				continue
			}
			region := []int{}
			pure := true
			stack := []int{q}
			seen[q] = true
			for len(stack) > 0 && len(region) <= 6 {
				p := stack[len(stack)-1]
				stack = stack[:len(stack)-1]
				region = append(region, p)
				for _, r := range b.Neighbors(p) {
					switch b.Points[r] {
					case Empty:
						if !seen[r] {
							seen[r] = true
							stack = append(stack, r)
						}
					case c.Opponent():
						pure = false
					}
				}
			}
			if pure && len(region) <= 3 {
				eyes++
			} else if pure && len(region) > 6 {
				eyes += 2 // a large own area: alive
			}
		}
	}
	return eyes
}
