package main

import (
	"fmt"
	"math"
	"strings"
)

// Astronomical periods in mean solar days, used as targets for gear trains.
const (
	siderealRate  = 1.00273790935 // sidereal days elapsed per mean solar day
	synodicMonth  = 29.530588853  // new moon to new moon
	draconicMonth = 27.212220817  // node to node; sets the eclipse seasons
	tropicalYear  = 365.24219

	minTeeth = 8 // smallest pinion the atelier will cut
)

// GearStage is one meshing pair: a driving wheel and the wheel it drives.
type GearStage struct {
	Driver int
	Driven int
}

// Train is a compound gear train fitted to an astronomical rate. The input is
// the movement's 24-hour wheel; Target is output turns per input turn.
type Train struct {
	Name    string
	Purpose string
	Target  float64
	Stages  []GearStage
}

// Ratio is the exact rate the cut teeth produce.
func (t Train) Ratio() float64 {
	r := 1.0
	for _, s := range t.Stages {
		r *= float64(s.Driver) / float64(s.Driven)
	}
	return r
}

// RelativeError is the fractional rate error against the astronomical target.
func (t Train) RelativeError() float64 {
	return (t.Ratio() - t.Target) / t.Target
}

// DriftSecondsPerDay is how far the indication gains (+) or loses (-) per day.
func (t Train) DriftSecondsPerDay() float64 {
	return t.RelativeError() * 86400
}

// YearsPerDayOfError is how long the train runs before the indication is a
// full day out.
func (t Train) YearsPerDayOfError() float64 {
	e := math.Abs(t.RelativeError())
	if e == 0 {
		return math.Inf(1)
	}
	return 1 / e / tropicalYear
}

// Notation renders the train the way it is written on the workshop drawing.
func (t Train) Notation() string {
	parts := make([]string, len(t.Stages))
	for i, s := range t.Stages {
		parts[i] = fmt.Sprintf("%d/%d", s.Driver, s.Driven)
	}
	return strings.Join(parts, " × ")
}

type trainKey struct {
	target   float64
	maxTeeth int
}

// TrainSolver searches tooth counts for two-stage trains and memoises the
// result, since several instruments share the same target and tooth limit.
type TrainSolver struct {
	cache map[trainKey][]GearStage
}

func NewTrainSolver() *TrainSolver {
	return &TrainSolver{cache: make(map[trainKey][]GearStage)}
}

func (s *TrainSolver) Solve(target float64, maxTeeth int) []GearStage {
	k := trainKey{target, maxTeeth}
	if st, ok := s.cache[k]; ok {
		return st
	}
	st := solveTwoStage(target, minTeeth, maxTeeth)
	s.cache[k] = st
	return st
}

// solveTwoStage finds the train (a/b) x (c/d) closest to target with every
// tooth count in [lo, hi]. Three counts are enumerated and the fourth derived,
// trying the integers either side of its ideal value: O(n^3), a few million
// iterations for the largest wheels the atelier cuts.
func solveTwoStage(target float64, lo, hi int) []GearStage {
	best := math.Inf(1)
	var stages []GearStage
	for a := lo; a <= hi; a++ {
		for b := lo; b <= hi; b++ {
			first := float64(a) / float64(b)
			for c := lo; c <= hi; c++ {
				ideal := first * float64(c) / target
				for _, d := range [2]int{int(math.Floor(ideal)), int(math.Ceil(ideal))} {
					if d < lo || d > hi {
						continue
					}
					if diff := math.Abs(first*float64(c)/float64(d) - target); diff < best {
						best = diff
						stages = []GearStage{{a, b}, {c, d}}
					}
				}
			}
		}
	}
	return stages
}

// trainsForTier returns the trains a complication tier carries, solved for
// the instrument's largest permissible wheel.
func trainsForTier(tierID string, maxTeeth int, s *TrainSolver) []Train {
	if maxTeeth < minTeeth {
		return nil
	}
	var out []Train
	if tierID == "horologium" || tierID == "uranographia" {
		out = append(out, Train{
			Name:    "Sidereal",
			Purpose: "Turns the rete once per sidereal day",
			Target:  siderealRate,
			Stages:  s.Solve(siderealRate, maxTeeth),
		})
	}
	if tierID == "uranographia" {
		out = append(out,
			Train{
				Name:    "Synodic",
				Purpose: "Turns the moon disc once per synodic month",
				Target:  1 / synodicMonth,
				Stages:  s.Solve(1/synodicMonth, maxTeeth),
			},
			Train{
				Name:    "Draconic",
				Purpose: "Carries the node pointer for eclipse seasons",
				Target:  1 / draconicMonth,
				Stages:  s.Solve(1/draconicMonth, maxTeeth),
			},
		)
	}
	return out
}
