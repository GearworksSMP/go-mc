package handler

import (
	"container/heap"
	"math"
)

// pathNode represents a node in the A* search.
type pathNode struct {
	x, y, z int
	g, h, f float64
	parent  *pathNode
	index   int // heap index
}

// nodeHeap implements heap.Interface for A* open set.
type nodeHeap []*pathNode

func (h nodeHeap) Len() int            { return len(h) }
func (h nodeHeap) Less(i, j int) bool   { return h[i].f < h[j].f }
func (h nodeHeap) Swap(i, j int)        { h[i], h[j] = h[j], h[i]; h[i].index = i; h[j].index = j }
func (h *nodeHeap) Push(x interface{})  { n := x.(*pathNode); n.index = len(*h); *h = append(*h, n) }
func (h *nodeHeap) Pop() interface{}    { old := *h; n := old[len(old)-1]; old[len(old)-1] = nil; n.index = -1; *h = old[:len(old)-1]; return n }

// PathStep is a single step along a computed path.
type PathStep struct {
	X, Y, Z int
}

// maxPathNodes limits the A* search to prevent lag spikes.
const maxPathNodes = 200

// pathfind runs A* from (sx,sy,sz) to (gx,gy,gz) using the world for walkability checks.
// Returns a slice of steps from start to goal (excluding start), or nil if no path found.
func (m *MobManager) pathfind(sx, sy, sz, gx, gy, gz int) []PathStep {
	type pos [3]int
	start := pos{sx, sy, sz}
	goal := pos{gx, gy, gz}

	if start == goal {
		return nil
	}

	startNode := &pathNode{x: sx, y: sy, z: sz}
	startNode.h = heuristic(sx, sy, sz, gx, gy, gz)
	startNode.f = startNode.h

	open := &nodeHeap{startNode}
	heap.Init(open)

	closed := make(map[pos]bool)
	inOpen := map[pos]*pathNode{start: startNode}

	explored := 0
	for open.Len() > 0 && explored < maxPathNodes {
		current := heap.Pop(open).(*pathNode)
		cp := pos{current.x, current.y, current.z}

		if cp == goal {
			return reconstructPath(current)
		}

		closed[cp] = true
		delete(inOpen, cp)
		explored++

		// Check 4 cardinal neighbors (+ step up/down)
		for _, d := range [][2]int{{1, 0}, {-1, 0}, {0, 1}, {0, -1}} {
			nx, nz := current.x+d[0], current.z+d[1]

			// Try same level, step up (+1), or step down (-1)
			for _, dy := range []int{0, 1, -1} {
				ny := current.y + dy

				np := pos{nx, ny, nz}
				if closed[np] {
					continue
				}

				// Walkability check: feet and head clear, ground below
				if !m.isWalkable(float64(nx)+0.5, float64(ny), float64(nz)+0.5) {
					continue
				}

				// For step up, check that head+1 at current position is clear (can jump)
				if dy == 1 {
					headAbove, err := m.World.GetBlock(current.x, current.y+2, current.z)
					if err != nil || isSolidBlock(headAbove) {
						continue
					}
				}

				moveCost := 1.0
				if dy != 0 {
					moveCost = 1.5 // penalize vertical movement
				}
				ng := current.g + moveCost

				if existing, ok := inOpen[np]; ok {
					if ng < existing.g {
						existing.g = ng
						existing.f = ng + existing.h
						existing.parent = current
						heap.Fix(open, existing.index)
					}
					continue
				}

				neighbor := &pathNode{
					x: nx, y: ny, z: nz,
					g:      ng,
					h:      heuristic(nx, ny, nz, gx, gy, gz),
					parent: current,
				}
				neighbor.f = neighbor.g + neighbor.h
				heap.Push(open, neighbor)
				inOpen[np] = neighbor
			}
		}
	}

	return nil // no path found within search limit
}

// heuristic returns the estimated distance (Euclidean) between two points.
func heuristic(x1, y1, z1, x2, y2, z2 int) float64 {
	dx := float64(x2 - x1)
	dy := float64(y2 - y1)
	dz := float64(z2 - z1)
	return math.Sqrt(dx*dx + dy*dy + dz*dz)
}

// reconstructPath builds the path from goal back to start via parent pointers.
// Returns steps from start to goal (excluding start).
func reconstructPath(goal *pathNode) []PathStep {
	var steps []PathStep
	for n := goal; n.parent != nil; n = n.parent {
		steps = append(steps, PathStep{X: n.x, Y: n.y, Z: n.z})
	}
	// Reverse to get start→goal order
	for i, j := 0, len(steps)-1; i < j; i, j = i+1, j-1 {
		steps[i], steps[j] = steps[j], steps[i]
	}
	return steps
}
