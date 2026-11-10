package check

import (
	"context"
	"strings"

	"github.com/Innocent-children/taskbelay/internal/ledger"
)

// DerivedKept 是每个起点保留的带测试识别结果的索引版本数；更早的版本需要时可以重建。
const DerivedKept = 2

/**
 * Prune 清理不再使用的分析数据：旧计算格式的索引和缓存、没有任务使用的起点索引、
 * 同一起点上过多的旧版本，以及没有索引引用的页。每一步都分小批提交，ctx 到期就停，
 * 下次从剩下的部分继续。全部清完后，空闲空间过半时收缩账本文件。
 */
func Prune(ctx context.Context, s *ledger.Store) error {
	indexes, err := s.AnalysisIndexes(ctx)
	if err != nil {
		return err
	}
	origins, err := s.TaskOrigins(ctx)
	if err != nil {
		return err
	}
	byID := map[string]ledger.IndexInfo{}
	keep := map[string]bool{}
	latestBase := map[string]string{}
	derived := map[string][]string{}
	for _, x := range indexes {
		byID[x.ID] = x
		if x.Variant != Algorithm && !strings.HasPrefix(x.Variant, Algorithm+":") {
			continue
		}
		if x.Variant == Algorithm {
			// 最近的起点索引留作下一个起点增量建立的基础。
			latestBase[x.ProjectID] = x.ID
		}
		if !origins[x.ProjectID][x.Tree] {
			continue
		}
		if x.Variant == Algorithm {
			keep[x.ID] = true
		} else {
			key := x.ProjectID + "\x00" + x.Tree
			derived[key] = append(derived[key], x.ID)
		}
	}
	for _, id := range latestBase {
		keep[id] = true
	}
	for _, ids := range derived {
		for _, id := range ids[max(0, len(ids)-DerivedKept):] {
			keep[id] = true
		}
	}
	// 索引按层保存差异，保留的版本所依赖的各层都要留下。
	for id := range keep {
		for parent := byID[id].ParentID; parent != "" && !keep[parent]; parent = byID[parent].ParentID {
			keep[parent] = true
		}
	}
	// 子版本先撤下，仍有已发布子版本的父层由账本保留。
	for i := len(indexes) - 1; i >= 0; i-- {
		x := indexes[i]
		if !keep[x.ID] {
			if err := s.RetireIndex(ctx, x.IndexBuild); err != nil {
				return err
			}
		}
	}
	pending, err := s.PendingIndexDeletions(ctx)
	if err != nil {
		return err
	}
	for _, build := range pending {
		for done := false; !done; {
			if done, err = s.DeleteIndexBatch(ctx, build); err != nil {
				return err
			}
		}
	}
	for done := false; !done; {
		if done, err = s.PurgeCacheBatch(ctx, Algorithm); err != nil {
			return err
		}
	}
	referenced, packed, revision, err := s.PageState(ctx)
	if err != nil {
		return err
	}
	for _, hash := range packed {
		part, err := s.PagePart(ctx, hash)
		if err != nil {
			return err
		}
		if part != "" {
			referenced[part] = true
		}
	}
	cached, err := s.CachedPages(ctx)
	if err != nil {
		return err
	}
	var orphans []string
	for _, hash := range cached {
		if !referenced[hash] {
			orphans = append(orphans, hash)
		}
	}
	for len(orphans) > 0 {
		n := min(len(orphans), PruneBatch)
		deleted, err := s.DeletePages(ctx, orphans[:n], revision)
		if err != nil {
			return err
		}
		if !deleted {
			// 其间有新的索引页登记，留到下次重新核对。
			return nil
		}
		orphans = orphans[n:]
	}
	return s.Shrink(ctx)
}
