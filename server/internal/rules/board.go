// Package rules implements the rules of Go: stone placement, captures, suicide, ko,
// and territory scoring with prisoners (Japanese-style counting, as on FlyOrDie).
package rules

import (
	"encoding/json"
	"errors"
)

// Color of a point on the board.
type Color int8

const (
	Empty Color = 0
	Black Color = 1
	White Color = 2
)

// Opponent returns the other player's color.
func (c Color) Opponent() Color {
	if c == Black {
		return White
	}
	if c == White {
		return Black
	}
	return Empty
}

// Letter is "b" or "w" (used in the wire protocol).
func (c Color) Letter() string {
	switch c {
	case Black:
		return "b"
	case White:
		return "w"
	}
	return ""
}

var (
	ErrOccupied = errors.New("point is occupied")
	ErrOutside  = errors.New("point is outside the board")
	ErrSuicide  = errors.New("suicide is not allowed")
	ErrKo       = errors.New("ko: the move would repeat the previous position")
)

// Board is a square Go board. Points are indexed y*Size+x.
type Board struct {
	Size   int
	Points []Color
	// koPoint is the single point where the opponent may not immediately recapture, or -1.
	koPoint int
}

// boardJSON is how a board is saved between server restarts (the ko point included).
type boardJSON struct {
	Size   int
	Points []Color
	Ko     int
}

func (b *Board) MarshalJSON() ([]byte, error) {
	return json.Marshal(boardJSON{Size: b.Size, Points: b.Points, Ko: b.koPoint})
}

func (b *Board) UnmarshalJSON(data []byte) error {
	var j boardJSON
	if err := json.Unmarshal(data, &j); err != nil {
		return err
	}
	b.Size, b.Points, b.koPoint = j.Size, j.Points, j.Ko
	return nil
}

// NewBoard creates an empty board.
func NewBoard(size int) *Board {
	return &Board{Size: size, Points: make([]Color, size*size), koPoint: -1}
}

// Clone returns an independent copy.
func (b *Board) Clone() *Board {
	c := &Board{Size: b.Size, Points: make([]Color, len(b.Points)), koPoint: b.koPoint}
	copy(c.Points, b.Points)
	return c
}

// Index converts coordinates to a point index.
func (b *Board) Index(x, y int) int { return y*b.Size + x }

// XY converts a point index to coordinates.
func (b *Board) XY(i int) (int, int) { return i % b.Size, i / b.Size }

// Neighbors returns the orthogonal neighbors of point i.
func (b *Board) Neighbors(i int) []int {
	x, y := b.XY(i)
	n := make([]int, 0, 4)
	if x > 0 {
		n = append(n, i-1)
	}
	if x < b.Size-1 {
		n = append(n, i+1)
	}
	if y > 0 {
		n = append(n, i-b.Size)
	}
	if y < b.Size-1 {
		n = append(n, i+b.Size)
	}
	return n
}

// Group returns the stones of the group containing point i and the number of its liberties.
func (b *Board) Group(i int) (stones []int, liberties int) {
	c := b.Points[i]
	if c == Empty {
		return nil, 0
	}
	seen := map[int]bool{i: true}
	libs := map[int]bool{}
	stack := []int{i}
	for len(stack) > 0 {
		p := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		stones = append(stones, p)
		for _, q := range b.Neighbors(p) {
			switch b.Points[q] {
			case Empty:
				libs[q] = true
			case c:
				if !seen[q] {
					seen[q] = true
					stack = append(stack, q)
				}
			}
		}
	}
	return stones, len(libs)
}

// Play places a stone of color c at (x, y), removes captured opponent stones and returns them.
// The board is unchanged if the move is illegal.
func (b *Board) Play(c Color, x, y int) ([]int, error) {
	if x < 0 || y < 0 || x >= b.Size || y >= b.Size {
		return nil, ErrOutside
	}
	i := b.Index(x, y)
	if b.Points[i] != Empty {
		return nil, ErrOccupied
	}
	if i == b.koPoint {
		return nil, ErrKo
	}
	b.Points[i] = c
	var captured []int
	seen := map[int]bool{}
	for _, q := range b.Neighbors(i) {
		if b.Points[q] != c.Opponent() || seen[q] {
			continue
		}
		stones, libs := b.Group(q)
		for _, s := range stones {
			seen[s] = true
		}
		if libs == 0 {
			captured = append(captured, stones...)
		}
	}
	for _, s := range captured {
		b.Points[s] = Empty
	}
	if len(captured) == 0 {
		if _, libs := b.Group(i); libs == 0 {
			b.Points[i] = Empty
			return nil, ErrSuicide
		}
	}
	// Simple ko: a single stone captured a single stone and now has exactly one liberty.
	b.koPoint = -1
	if len(captured) == 1 {
		if stones, libs := b.Group(i); len(stones) == 1 && libs == 1 {
			b.koPoint = captured[0]
		}
	}
	return captured, nil
}

// Pass clears the ko restriction (a pass resolves the ko).
func (b *Board) Pass() { b.koPoint = -1 }

// String encodes the board as one character per point: '.', 'b', 'w'.
func (b *Board) String() string {
	out := make([]byte, len(b.Points))
	for i, p := range b.Points {
		switch p {
		case Black:
			out[i] = 'b'
		case White:
			out[i] = 'w'
		default:
			out[i] = '.'
		}
	}
	return string(out)
}
