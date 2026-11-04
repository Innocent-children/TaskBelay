package changes

import (
	"container/heap"
	"context"
	"math"
	"sort"
)

func Push(ctx context.Context, g Graph, seeds map[string]float64) (map[string]float64, int, error) {
	scores, residual := map[string]float64{}, map[string]float64{}
	sum := 0.0
	for _, key := range sortedKeys(seeds) {
		sum += seeds[key]
	}
	if sum == 0 {
		return scores, 0, nil
	}
	var queue []string
	queued := map[string]bool{}
	for _, key := range sortedKeys(seeds) {
		residual[key] = seeds[key] / sum
		queue = append(queue, key)
		queued[key] = true
	}
	pushes := 0
	for head := 0; head < len(queue); head++ {
		if err := ctx.Err(); err != nil {
			return scores, pushes, err
		}
		v := queue[head]
		queued[v] = false
		degree, err := g.degree(ctx, v)
		if err != nil {
			return scores, pushes, err
		}
		mass := residual[v]
		if degree == 0 {
			scores[v] += mass
			residual[v] = 0
			continue
		}
		if mass < Epsilon*degree {
			continue
		}
		pushes++
		edges, err := g.neighbours(ctx, v)
		if err != nil {
			return scores, pushes, err
		}
		scores[v] += Alpha * mass
		residual[v] = (1 - Alpha) * mass / 2
		for _, w := range sortedKeys(edges) {
			residual[w] += (1 - Alpha) * mass * edges[w] / (2 * degree)
			d, err := g.degree(ctx, w)
			if err != nil {
				return scores, pushes, err
			}
			if residual[w] >= Epsilon*d && !queued[w] {
				queue = append(queue, w)
				queued[w] = true
			}
		}
		if residual[v] >= Epsilon*degree && !queued[v] {
			queue = append(queue, v)
			queued[v] = true
		}
	}
	return scores, pushes, nil
}

func Sweep(ctx context.Context, g Graph, scores map[string]float64) ([]string, float64, error) {
	nodes := sortedKeys(scores)
	for _, n := range nodes {
		if _, err := g.degree(ctx, n); err != nil {
			return nil, 0, err
		}
	}
	sort.Slice(nodes, func(i, j int) bool {
		a, b := scores[nodes[i]]/math.Max(Epsilon, g.Degree[nodes[i]]), scores[nodes[j]]/math.Max(Epsilon, g.Degree[nodes[j]])
		if a == b {
			return nodes[i] < nodes[j]
		}
		return a > b
	})
	inside := map[string]bool{}
	volume, cut, best := 0.0, 0.0, math.Inf(1)
	count := 0
	for i, v := range nodes {
		inside[v] = true
		volume += g.Degree[v]
		edges, err := g.neighbours(ctx, v)
		if err != nil {
			return nil, 0, err
		}
		for _, w := range sortedKeys(edges) {
			weight := edges[w]
			if inside[w] {
				cut -= weight
			} else {
				cut += weight
			}
		}
		denom := math.Min(volume, g.Volume-volume)
		if denom <= Epsilon {
			continue
		}
		phi := math.Max(0, cut) / denom
		if phi < best {
			best = phi
			count = i + 1
		}
	}
	if count == 0 && len(nodes) > 0 {
		count = 1
		best = 0
	}
	result := append([]string(nil), nodes[:count]...)
	sort.Strings(result)
	return result, best, nil
}

type distance struct {
	node  string
	value float64
}
type distances []distance

func (q distances) Len() int { return len(q) }
func (q distances) Less(i, j int) bool {
	if q[i].value == q[j].value {
		return q[i].node < q[j].node
	}
	return q[i].value < q[j].value
}
func (q distances) Swap(i, j int) { q[i], q[j] = q[j], q[i] }
func (q *distances) Push(x any)   { *q = append(*q, x.(distance)) }
func (q *distances) Pop() any     { old := *q; x := old[len(old)-1]; *q = old[:len(old)-1]; return x }

func Strongest(ctx context.Context, g Graph, seeds map[string]float64, clusters []Cluster) (map[string][]string, error) {
	dist := map[string]float64{}
	previous := map[string]string{}
	q := &distances{}
	for _, node := range sortedKeys(seeds) {
		dist[node] = 0
		heap.Push(q, distance{node, 0})
	}
	targets := map[string]bool{}
	for _, c := range clusters {
		targets["c:"+c.ID] = true
	}
	remaining := len(targets)
	for q.Len() > 0 && remaining > 0 {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		d := heap.Pop(q).(distance)
		if d.value != dist[d.node] {
			continue
		}
		if targets[d.node] {
			delete(targets, d.node)
			remaining--
		}
		for _, n := range sortedKeys(g.Edges[d.node]) {
			// 路径解释只使用推送已触及的邻域，不能为不可达的簇扩展到整个仓库。
			if _, known := g.Edges[n]; !known {
				continue
			}
			candidate := d.value - math.Log(g.Edges[d.node][n]/g.Degree[d.node])
			old, ok := dist[n]
			if !ok || candidate < old {
				dist[n] = candidate
				previous[n] = d.node
				heap.Push(q, distance{n, candidate})
			}
		}
	}
	out := map[string][]string{}
	for _, c := range clusters {
		node := "c:" + c.ID
		if _, ok := dist[node]; !ok {
			continue
		}
		var path []string
		for node != "" {
			path = append(path, node)
			node = previous[node]
		}
		for i, j := 0, len(path)-1; i < j; i, j = i+1, j-1 {
			path[i], path[j] = path[j], path[i]
		}
		out[c.ID] = path
	}
	return out, nil
}
