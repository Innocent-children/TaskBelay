package changes

import (
	"encoding/binary"
	"encoding/hex"
	"math"
)

// VolumeMoment 保存边权的系数；达到 epsilon 的单元数作为有序前缀键。
type VolumeMoment struct{ A, B float64 }
type VolumeNode map[byte]VolumeMoment

func volumeKey(n float64) []byte {
	var b [8]byte
	binary.BigEndian.PutUint64(b[:], math.Float64bits(n))
	return b[:]
}

/**
 * 每个词按出现次数分组；单元总数改变时通过八层前缀汇总求体积，不遍历词表。
 */
func AdjustVolume(df, count, multiplicity int, read func(string) VolumeNode, write func(string, VolumeNode)) {
	if df <= 0 || count <= 0 || multiplicity == 0 {
		return
	}
	a := math.Log1p(float64(count))
	key := volumeKey(float64(df) * math.Exp(Epsilon/a))
	m := VolumeMoment{float64(multiplicity) * a, float64(multiplicity) * a * math.Log(float64(df))}
	for i, b := range key {
		p := hex.EncodeToString(key[:i])
		node := read(p)
		if node == nil {
			node = VolumeNode{}
		}
		v := node[b]
		v.A += m.A
		v.B += m.B
		node[b] = v
		write(p, node)
	}
}

func GraphVolume(total int, read func(string) VolumeNode) float64 {
	key := volumeKey(float64(max(1, total)))
	sum := VolumeMoment{}
	for i, b := range key {
		node := read(hex.EncodeToString(key[:i]))
		for c, v := range node {
			if c < b || i == len(key)-1 && c == b {
				sum.A += v.A
				sum.B += v.B
			}
		}
	}
	return 2 * (math.Max(0, math.Log(float64(max(1, total)))*sum.A-sum.B) + float64(total)*FileWeight)
}

func WordWeight(total, df, count int) float64 {
	if df <= 0 || count <= 0 {
		return 0
	}
	w := math.Log(float64(max(1, total))/float64(df)) * math.Log1p(float64(count))
	if w < Epsilon {
		return 0
	}
	return w
}
