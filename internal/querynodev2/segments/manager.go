// Licensed to the LF AI & Data foundation under one
// or more contributor license agreements. See the NOTICE file
// distributed with this work for additional information
// regarding copyright ownership. The ASF licenses this file
// to you under the Apache License, Version 2.0 (the
// "License"); you may not use this file except in compliance
// with the License. You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package segments

/*
#cgo pkg-config: milvus_segcore

#include "segcore/collection_c.h"
#include "segcore/segment_c.h"
*/
import "C"

import (
	"context"
	"fmt"
	"sync"

	"go.uber.org/zap"
	"golang.org/x/sync/singleflight"

	"github.com/milvus-io/milvus-proto/go-api/v2/commonpb"
	"github.com/milvus-io/milvus/internal/proto/datapb"
	"github.com/milvus-io/milvus/internal/proto/querypb"
	"github.com/milvus-io/milvus/pkg/eventlog"
	"github.com/milvus-io/milvus/pkg/log"
	"github.com/milvus-io/milvus/pkg/metrics"
	"github.com/milvus-io/milvus/pkg/util/cache"
	"github.com/milvus-io/milvus/pkg/util/merr"
	"github.com/milvus-io/milvus/pkg/util/paramtable"
	"github.com/milvus-io/milvus/pkg/util/typeutil"
)

// SegmentFilter is the interface for segment selection criteria.
type SegmentFilter interface {
	Filter(segment Segment) bool
	SegmentType() (SegmentType, bool)
	SegmentIDs() ([]int64, bool)
}

// SegmentFilterFunc is a type wrapper for `func(Segment) bool` to SegmentFilter.
type SegmentFilterFunc func(segment Segment) bool

func (f SegmentFilterFunc) Filter(segment Segment) bool {
	return f(segment)
}

func (f SegmentFilterFunc) SegmentType() (SegmentType, bool) {
	return commonpb.SegmentState_SegmentStateNone, false
}

func (s SegmentFilterFunc) SegmentIDs() ([]int64, bool) {
	return nil, false
}

// SegmentIDFilter is the specific segment filter for SegmentID only.
type SegmentIDFilter int64

func (f SegmentIDFilter) Filter(segment Segment) bool {
	return segment.ID() == int64(f)
}

func (f SegmentIDFilter) SegmentType() (SegmentType, bool) {
	return commonpb.SegmentState_SegmentStateNone, false
}

func (f SegmentIDFilter) SegmentIDs() ([]int64, bool) {
	return []int64{int64(f)}, true
}

type SegmentTypeFilter SegmentType

func (f SegmentTypeFilter) Filter(segment Segment) bool {
	return segment.Type() == SegmentType(f)
}

func (f SegmentTypeFilter) SegmentType() (SegmentType, bool) {
	return SegmentType(f), true
}

func (f SegmentTypeFilter) SegmentIDs() ([]int64, bool) {
	return nil, false
}

func WithSkipEmpty() SegmentFilter {
	return SegmentFilterFunc(func(segment Segment) bool {
		return segment.InsertCount() > 0
	})
}

func WithPartition(partitionID typeutil.UniqueID) SegmentFilter {
	return SegmentFilterFunc(func(segment Segment) bool {
		return segment.Partition() == partitionID
	})
}

func WithChannel(channel string) SegmentFilter {
	return SegmentFilterFunc(func(segment Segment) bool {
		return segment.Shard() == channel
	})
}

func WithType(typ SegmentType) SegmentFilter {
	return SegmentTypeFilter(typ)
}

func WithID(id int64) SegmentFilter {
	return SegmentIDFilter(id)
}

func WithLevel(level datapb.SegmentLevel) SegmentFilter {
	return SegmentFilterFunc(func(segment Segment) bool {
		return segment.Level() == level
	})
}

type SegmentAction func(segment Segment) bool

func IncreaseVersion(version int64) SegmentAction {
	return func(segment Segment) bool {
		log := log.Ctx(context.Background()).With(
			zap.Int64("segmentID", segment.ID()),
			zap.String("type", segment.Type().String()),
			zap.Int64("segmentVersion", segment.Version()),
			zap.Int64("updateVersion", version),
		)
		for oldVersion := segment.Version(); oldVersion < version; {
			if segment.CASVersion(oldVersion, version) {
				return true
			}
		}
		log.Warn("segment version cannot go backwards, skip update")
		return false
	}
}

type actionType int32

const (
	removeAction actionType = iota
	addAction
)

type Manager struct {
	Collection CollectionManager
	Segment    SegmentManager
	DiskCache  cache.Cache[int64, Segment]
}

func NewManager() *Manager {
	diskCap := paramtable.Get().QueryNodeCfg.DiskCapacityLimit.GetAsInt64()

	segMgr := NewSegmentManager()
	sf := singleflight.Group{}
	manager := &Manager{
		Collection: NewCollectionManager(),
		Segment:    segMgr,
	}

	manager.DiskCache = cache.NewCacheBuilder[int64, Segment]().WithLazyScavenger(func(key int64) int64 {
		return int64(segMgr.sealedSegments[key].ResourceUsageEstimate().DiskSize)
	}, diskCap).WithLoader(func(key int64) (Segment, bool) {
		log.Debug("cache missed segment", zap.Int64("segmentID", key))
		segMgr.mu.RLock()
		defer segMgr.mu.RUnlock()

		segment, ok := segMgr.sealedSegments[key]
		if !ok {
			// the segment has been released, just ignore it
			return nil, false
		}

		info := segment.LoadInfo()
		_, err, _ := sf.Do(fmt.Sprint(segment.ID()), func() (interface{}, error) {
			collection := manager.Collection.Get(segment.Collection())
			if collection == nil {
				return nil, merr.WrapErrCollectionNotLoaded(segment.Collection(), "failed to load segment fields")
			}
			err := loadSealedSegmentFields(context.Background(), collection, segment.(*LocalSegment), info.BinlogPaths, info.GetNumOfRows(), WithLoadStatus(LoadStatusMapped))
			return nil, err
		})
		if err != nil {
			log.Warn("cache sealed segment failed", zap.Error(err))
			return nil, false
		}
		return segment, true
	}).WithFinalizer(func(key int64, segment Segment) error {
		log.Debug("evict segment from cache", zap.Int64("segmentID", key))
		segment.Release(WithReleaseScope(ReleaseScopeData))
		return nil
	}).Build()
	return manager
}

type SegmentManager interface {
	// Put puts the given segments in,
	// and increases the ref count of the corresponding collection,
	// dup segments will not increase the ref count
	Put(segmentType SegmentType, segments ...Segment)
	UpdateBy(action SegmentAction, filters ...SegmentFilter) int
	Get(segmentID typeutil.UniqueID) Segment
	GetWithType(segmentID typeutil.UniqueID, typ SegmentType) Segment
	GetBy(filters ...SegmentFilter) []Segment
	// Get segments and acquire the read locks
	GetAndPinBy(filters ...SegmentFilter) ([]Segment, error)
	GetAndPin(segments []int64, filters ...SegmentFilter) ([]Segment, error)
	Unpin(segments []Segment)

	GetSealed(segmentID typeutil.UniqueID) Segment
	GetGrowing(segmentID typeutil.UniqueID) Segment
	Empty() bool

	// Remove removes the given segment,
	// and decreases the ref count of the corresponding collection,
	// will not decrease the ref count if the given segment not exists
	Remove(segmentID typeutil.UniqueID, scope querypb.DataScope) (int, int)
	RemoveBy(filters ...SegmentFilter) (int, int)
	Clear()
}

var _ SegmentManager = (*segmentManager)(nil)

// Manager manages all collections and segments
type segmentManager struct {
	mu sync.RWMutex // guards all

	growingSegments map[typeutil.UniqueID]Segment
	sealedSegments  map[typeutil.UniqueID]Segment
}

func NewSegmentManager() *segmentManager {
	mgr := &segmentManager{
		growingSegments: make(map[int64]Segment),
		sealedSegments:  make(map[int64]Segment),
	}
	return mgr
}

func (mgr *segmentManager) Put(segmentType SegmentType, segments ...Segment) {
	var replacedSegment []Segment
	mgr.mu.Lock()
	defer mgr.mu.Unlock()
	var targetMap map[int64]Segment
	switch segmentType {
	case SegmentTypeGrowing:
		targetMap = mgr.growingSegments
	case SegmentTypeSealed:
		targetMap = mgr.sealedSegments
	default:
		panic("unexpected segment type")
	}

	for _, segment := range segments {
		oldSegment, ok := targetMap[segment.ID()]

		if ok {
			if oldSegment.Version() >= segment.Version() {
				log.Warn("Invalid segment distribution changed, skip it",
					zap.Int64("segmentID", segment.ID()),
					zap.Int64("oldVersion", oldSegment.Version()),
					zap.Int64("newVersion", segment.Version()),
				)
				// delete redundant segment
				segment.Release()
				continue
			}
			replacedSegment = append(replacedSegment, oldSegment)
		}
		targetMap[segment.ID()] = segment

		eventlog.Record(eventlog.NewRawEvt(eventlog.Level_Info, fmt.Sprintf("Segment %d[%d] loaded", segment.ID(), segment.Collection())))
		metrics.QueryNodeNumSegments.WithLabelValues(
			fmt.Sprint(paramtable.GetNodeID()),
			fmt.Sprint(segment.Collection()),
			fmt.Sprint(segment.Partition()),
			segment.Type().String(),
			fmt.Sprint(len(segment.Indexes())),
			segment.Level().String(),
		).Inc()
	}
	mgr.updateMetric()

	// release replaced segment
	if len(replacedSegment) > 0 {
		go func() {
			for _, segment := range replacedSegment {
				remove(segment)
			}
		}()
	}
}

func (mgr *segmentManager) UpdateBy(action SegmentAction, filters ...SegmentFilter) int {
	mgr.mu.RLock()
	defer mgr.mu.RUnlock()

	updated := 0
	mgr.rangeWithFilter(func(_ int64, _ SegmentType, segment Segment) bool {
		if action(segment) {
			updated++
		}
		return true
	}, filters...)
	return updated
}

func (mgr *segmentManager) Get(segmentID typeutil.UniqueID) Segment {
	mgr.mu.RLock()
	defer mgr.mu.RUnlock()

	if segment, ok := mgr.growingSegments[segmentID]; ok {
		return segment
	} else if segment, ok = mgr.sealedSegments[segmentID]; ok {
		return segment
	}

	return nil
}

func (mgr *segmentManager) GetWithType(segmentID typeutil.UniqueID, typ SegmentType) Segment {
	mgr.mu.RLock()
	defer mgr.mu.RUnlock()

	switch typ {
	case SegmentTypeSealed:
		return mgr.sealedSegments[segmentID]
	case SegmentTypeGrowing:
		return mgr.growingSegments[segmentID]
	default:
		return nil
	}
}

func (mgr *segmentManager) GetBy(filters ...SegmentFilter) []Segment {
	mgr.mu.RLock()
	defer mgr.mu.RUnlock()

	var ret []Segment
	mgr.rangeWithFilter(func(id int64, _ SegmentType, segment Segment) bool {
		ret = append(ret, segment)
		return true
	}, filters...)
	return ret
}

func (mgr *segmentManager) GetAndPinBy(filters ...SegmentFilter) ([]Segment, error) {
	mgr.mu.RLock()
	defer mgr.mu.RUnlock()

	var ret []Segment
	var err error
	defer func() {
		if err != nil {
			for _, segment := range ret {
				segment.RUnlock()
			}
		}
	}()

	mgr.rangeWithFilter(func(id int64, _ SegmentType, segment Segment) bool {
		if segment.Level() == datapb.SegmentLevel_L0 {
			return true
		}
		err = segment.RLock()
		if err != nil {
			return false
		}
		ret = append(ret, segment)
		return true
	}, filters...)

	return ret, nil
}

func (mgr *segmentManager) GetAndPin(segments []int64, filters ...SegmentFilter) ([]Segment, error) {
	mgr.mu.RLock()
	defer mgr.mu.RUnlock()

	lockedSegments := make([]Segment, 0, len(segments))
	var err error
	defer func() {
		if err != nil {
			for _, segment := range lockedSegments {
				segment.RUnlock()
			}
		}
	}()

	for _, id := range segments {
		growing, growingExist := mgr.growingSegments[id]
		sealed, sealedExist := mgr.sealedSegments[id]

		// L0 Segment should not be queryable.
		if sealedExist && sealed.Level() == datapb.SegmentLevel_L0 {
			continue
		}

		growingExist = growingExist && filter(growing, filters...)
		sealedExist = sealedExist && filter(sealed, filters...)

		if growingExist {
			err = growing.RLock()
			if err != nil {
				return nil, err
			}
			lockedSegments = append(lockedSegments, growing)
		}
		if sealedExist {
			err = sealed.RLock()
			if err != nil {
				return nil, err
			}
			lockedSegments = append(lockedSegments, sealed)
		}

		if !growingExist && !sealedExist {
			err = merr.WrapErrSegmentNotLoaded(id, "segment not found")
			return nil, err
		}
	}

	return lockedSegments, nil
}

func (mgr *segmentManager) Unpin(segments []Segment) {
	for _, segment := range segments {
		segment.RUnlock()
	}
}

func (mgr *segmentManager) rangeWithFilter(process func(id int64, segType SegmentType, segment Segment) bool, filters ...SegmentFilter) {
	var segType SegmentType
	var hasSegType, hasSegIDs bool
	segmentIDs := typeutil.NewSet[int64]()

	otherFilters := make([]SegmentFilter, 0, len(filters))
	for _, filter := range filters {
		if sType, ok := filter.SegmentType(); ok {
			segType = sType
			hasSegType = true
			continue
		}
		if segIDs, ok := filter.SegmentIDs(); ok {
			hasSegIDs = true
			segmentIDs.Insert(segIDs...)
			continue
		}
		otherFilters = append(otherFilters, filter)
	}

	mergedFilter := func(info Segment) bool {
		for _, filter := range otherFilters {
			if !filter.Filter(info) {
				return false
			}
		}
		return true
	}

	var candidates map[SegmentType]map[int64]Segment
	switch segType {
	case SegmentTypeSealed:
		candidates = map[SegmentType]map[int64]Segment{SegmentTypeSealed: mgr.sealedSegments}
	case SegmentTypeGrowing:
		candidates = map[SegmentType]map[int64]Segment{SegmentTypeGrowing: mgr.growingSegments}
	default:
		if !hasSegType {
			candidates = map[SegmentType]map[int64]Segment{
				SegmentTypeSealed:  mgr.sealedSegments,
				SegmentTypeGrowing: mgr.growingSegments,
			}
		}
	}

	for segType, candidate := range candidates {
		if hasSegIDs {
			for id := range segmentIDs {
				segment, has := candidate[id]
				if has && mergedFilter(segment) {
					if !process(id, segType, segment) {
						break
					}
				}
			}
		} else {
			for id, segment := range candidate {
				if mergedFilter(segment) {
					if !process(id, segType, segment) {
						break
					}
				}
			}
		}
	}
}

func filter(segment Segment, filters ...SegmentFilter) bool {
	for _, filter := range filters {
		if !filter.Filter(segment) {
			return false
		}
	}
	return true
}

func (mgr *segmentManager) GetSealed(segmentID typeutil.UniqueID) Segment {
	mgr.mu.RLock()
	defer mgr.mu.RUnlock()

	if segment, ok := mgr.sealedSegments[segmentID]; ok {
		return segment
	}

	return nil
}

func (mgr *segmentManager) GetGrowing(segmentID typeutil.UniqueID) Segment {
	mgr.mu.RLock()
	defer mgr.mu.RUnlock()

	if segment, ok := mgr.growingSegments[segmentID]; ok {
		return segment
	}

	return nil
}

func (mgr *segmentManager) Empty() bool {
	mgr.mu.RLock()
	defer mgr.mu.RUnlock()

	return len(mgr.growingSegments)+len(mgr.sealedSegments) == 0
}

// returns true if the segment exists,
// false otherwise
func (mgr *segmentManager) Remove(segmentID typeutil.UniqueID, scope querypb.DataScope) (int, int) {
	mgr.mu.Lock()

	var removeGrowing, removeSealed int
	var growing, sealed Segment
	switch scope {
	case querypb.DataScope_Streaming:
		growing = mgr.removeSegmentWithType(SegmentTypeGrowing, segmentID)
		if growing != nil {
			removeGrowing = 1
		}

	case querypb.DataScope_Historical:
		sealed = mgr.removeSegmentWithType(SegmentTypeSealed, segmentID)
		if sealed != nil {
			removeSealed = 1
		}

	case querypb.DataScope_All:
		growing = mgr.removeSegmentWithType(SegmentTypeGrowing, segmentID)
		if growing != nil {
			removeGrowing = 1
		}

		sealed = mgr.removeSegmentWithType(SegmentTypeSealed, segmentID)
		if sealed != nil {
			removeSealed = 1
		}
	}
	mgr.updateMetric()
	mgr.mu.Unlock()

	if growing != nil {
		remove(growing)
	}

	if sealed != nil {
		remove(sealed)
	}

	return removeGrowing, removeSealed
}

func (mgr *segmentManager) removeSegmentWithType(typ SegmentType, segmentID typeutil.UniqueID) Segment {
	switch typ {
	case SegmentTypeGrowing:
		s, ok := mgr.growingSegments[segmentID]
		if ok {
			delete(mgr.growingSegments, segmentID)
			return s
		}

	case SegmentTypeSealed:
		s, ok := mgr.sealedSegments[segmentID]
		if ok {
			delete(mgr.sealedSegments, segmentID)
			return s
		}
	default:
		return nil
	}

	return nil
}

func (mgr *segmentManager) RemoveBy(filters ...SegmentFilter) (int, int) {
	mgr.mu.Lock()

	var removeSegments []Segment
	var removeGrowing, removeSealed int

	mgr.rangeWithFilter(func(id int64, segType SegmentType, segment Segment) bool {
		s := mgr.removeSegmentWithType(segType, id)
		if s != nil {
			removeSegments = append(removeSegments, s)
			switch segType {
			case SegmentTypeGrowing:
				removeGrowing++
			case SegmentTypeSealed:
				removeSealed++
			}
		}
		return true
	}, filters...)
	mgr.updateMetric()
	mgr.mu.Unlock()

	for _, s := range removeSegments {
		remove(s)
	}

	return removeGrowing, removeSealed
}

func (mgr *segmentManager) Clear() {
	mgr.mu.Lock()
	defer mgr.mu.Unlock()

	for id, segment := range mgr.growingSegments {
		delete(mgr.growingSegments, id)
		remove(segment)
	}

	for id, segment := range mgr.sealedSegments {
		delete(mgr.sealedSegments, id)
		remove(segment)
	}
	mgr.updateMetric()
}

func (mgr *segmentManager) updateMetric() {
	// update collection and partiation metric
	collections, partiations := make(typeutil.Set[int64]), make(typeutil.Set[int64])
	for _, seg := range mgr.growingSegments {
		collections.Insert(seg.Collection())
		partiations.Insert(seg.Partition())
	}
	for _, seg := range mgr.sealedSegments {
		collections.Insert(seg.Collection())
		partiations.Insert(seg.Partition())
	}
	metrics.QueryNodeNumCollections.WithLabelValues(fmt.Sprint(paramtable.GetNodeID())).Set(float64(collections.Len()))
	metrics.QueryNodeNumPartitions.WithLabelValues(fmt.Sprint(paramtable.GetNodeID())).Set(float64(partiations.Len()))
}

func remove(segment Segment) bool {
	segment.Release()

	metrics.QueryNodeNumSegments.WithLabelValues(
		fmt.Sprint(paramtable.GetNodeID()),
		fmt.Sprint(segment.Collection()),
		fmt.Sprint(segment.Partition()),
		segment.Type().String(),
		fmt.Sprint(len(segment.Indexes())),
		segment.Level().String(),
	).Dec()

	return true
}
