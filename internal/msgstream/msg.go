package msgstream

import (
	"context"

	"github.com/golang/protobuf/proto"
	internalPb "github.com/zilliztech/milvus-distributed/internal/proto/internalpb"
)

type MsgType = internalPb.MsgType

type TsMsg interface {
	GetMsgContext() context.Context
	SetMsgContext(context.Context)
	BeginTs() Timestamp
	EndTs() Timestamp
	Type() MsgType
	HashKeys() []uint32
	Marshal(TsMsg) ([]byte, error)
	Unmarshal([]byte) (TsMsg, error)
}

type BaseMsg struct {
	MsgCtx         context.Context
	BeginTimestamp Timestamp
	EndTimestamp   Timestamp
	HashValues     []uint32
}

func (bm *BaseMsg) BeginTs() Timestamp {
	return bm.BeginTimestamp
}

func (bm *BaseMsg) EndTs() Timestamp {
	return bm.EndTimestamp
}

func (bm *BaseMsg) HashKeys() []uint32 {
	return bm.HashValues
}

/////////////////////////////////////////Insert//////////////////////////////////////////
type InsertMsg struct {
	BaseMsg
	internalPb.InsertRequest
}

func (it *InsertMsg) Type() MsgType {
	return it.MsgType
}

func (it *InsertMsg) GetMsgContext() context.Context {
	return it.MsgCtx
}

func (it *InsertMsg) SetMsgContext(ctx context.Context) {
	it.MsgCtx = ctx
}

func (it *InsertMsg) Marshal(input TsMsg) ([]byte, error) {
	insertMsg := input.(*InsertMsg)
	insertRequest := &insertMsg.InsertRequest
	mb, err := proto.Marshal(insertRequest)
	if err != nil {
		return nil, err
	}
	return mb, nil
}

func (it *InsertMsg) Unmarshal(input []byte) (TsMsg, error) {
	insertRequest := internalPb.InsertRequest{}
	err := proto.Unmarshal(input, &insertRequest)
	if err != nil {
		return nil, err
	}
	insertMsg := &InsertMsg{InsertRequest: insertRequest}
	for _, timestamp := range insertMsg.Timestamps {
		insertMsg.BeginTimestamp = timestamp
		insertMsg.EndTimestamp = timestamp
		break
	}
	for _, timestamp := range insertMsg.Timestamps {
		if timestamp > insertMsg.EndTimestamp {
			insertMsg.EndTimestamp = timestamp
		}
		if timestamp < insertMsg.BeginTimestamp {
			insertMsg.BeginTimestamp = timestamp
		}
	}

	return insertMsg, nil
}

/////////////////////////////////////////Flush//////////////////////////////////////////
type FlushMsg struct {
	BaseMsg
	internalPb.FlushMsg
}

func (fl *FlushMsg) Type() MsgType {
	return fl.GetMsgType()
}

func (fl *FlushMsg) GetMsgContext() context.Context {
	return fl.MsgCtx
}
func (fl *FlushMsg) SetMsgContext(ctx context.Context) {
	fl.MsgCtx = ctx
}

func (fl *FlushMsg) Marshal(input TsMsg) ([]byte, error) {
	flushMsgTask := input.(*FlushMsg)
	flushMsg := &flushMsgTask.FlushMsg
	mb, err := proto.Marshal(flushMsg)
	if err != nil {
		return nil, err
	}
	return mb, nil
}

func (fl *FlushMsg) Unmarshal(input []byte) (TsMsg, error) {
	flushMsg := internalPb.FlushMsg{}
	err := proto.Unmarshal(input, &flushMsg)
	if err != nil {
		return nil, err
	}
	flushMsgTask := &FlushMsg{FlushMsg: flushMsg}
	flushMsgTask.BeginTimestamp = flushMsgTask.Timestamp
	flushMsgTask.EndTimestamp = flushMsgTask.Timestamp

	return flushMsgTask, nil
}

/////////////////////////////////////////Delete//////////////////////////////////////////
type DeleteMsg struct {
	BaseMsg
	internalPb.DeleteRequest
}

func (dt *DeleteMsg) Type() MsgType {
	return dt.MsgType
}

func (dt *DeleteMsg) GetMsgContext() context.Context {
	return dt.MsgCtx
}

func (dt *DeleteMsg) SetMsgContext(ctx context.Context) {
	dt.MsgCtx = ctx
}

func (dt *DeleteMsg) Marshal(input TsMsg) ([]byte, error) {
	deleteTask := input.(*DeleteMsg)
	deleteRequest := &deleteTask.DeleteRequest
	mb, err := proto.Marshal(deleteRequest)
	if err != nil {
		return nil, err
	}
	return mb, nil
}

func (dt *DeleteMsg) Unmarshal(input []byte) (TsMsg, error) {
	deleteRequest := internalPb.DeleteRequest{}
	err := proto.Unmarshal(input, &deleteRequest)
	if err != nil {
		return nil, err
	}
	deleteMsg := &DeleteMsg{DeleteRequest: deleteRequest}
	for _, timestamp := range deleteMsg.Timestamps {
		deleteMsg.BeginTimestamp = timestamp
		deleteMsg.EndTimestamp = timestamp
		break
	}
	for _, timestamp := range deleteMsg.Timestamps {
		if timestamp > deleteMsg.EndTimestamp {
			deleteMsg.EndTimestamp = timestamp
		}
		if timestamp < deleteMsg.BeginTimestamp {
			deleteMsg.BeginTimestamp = timestamp
		}
	}

	return deleteMsg, nil
}

/////////////////////////////////////////Search//////////////////////////////////////////
type SearchMsg struct {
	BaseMsg
	internalPb.SearchRequest
}

func (st *SearchMsg) Type() MsgType {
	return st.MsgType
}

func (st *SearchMsg) GetMsgContext() context.Context {
	return st.MsgCtx
}

func (st *SearchMsg) SetMsgContext(ctx context.Context) {
	st.MsgCtx = ctx
}

func (st *SearchMsg) Marshal(input TsMsg) ([]byte, error) {
	searchTask := input.(*SearchMsg)
	searchRequest := &searchTask.SearchRequest
	mb, err := proto.Marshal(searchRequest)
	if err != nil {
		return nil, err
	}
	return mb, nil
}

func (st *SearchMsg) Unmarshal(input []byte) (TsMsg, error) {
	searchRequest := internalPb.SearchRequest{}
	err := proto.Unmarshal(input, &searchRequest)
	if err != nil {
		return nil, err
	}
	searchMsg := &SearchMsg{SearchRequest: searchRequest}
	searchMsg.BeginTimestamp = searchMsg.Timestamp
	searchMsg.EndTimestamp = searchMsg.Timestamp

	return searchMsg, nil
}

/////////////////////////////////////////SearchResult//////////////////////////////////////////
type SearchResultMsg struct {
	BaseMsg
	internalPb.SearchResult
}

func (srt *SearchResultMsg) Type() MsgType {
	return srt.MsgType
}

func (srt *SearchResultMsg) GetMsgContext() context.Context {
	return srt.MsgCtx
}

func (srt *SearchResultMsg) SetMsgContext(ctx context.Context) {
	srt.MsgCtx = ctx
}

func (srt *SearchResultMsg) Marshal(input TsMsg) ([]byte, error) {
	searchResultTask := input.(*SearchResultMsg)
	searchResultRequest := &searchResultTask.SearchResult
	mb, err := proto.Marshal(searchResultRequest)
	if err != nil {
		return nil, err
	}
	return mb, nil
}

func (srt *SearchResultMsg) Unmarshal(input []byte) (TsMsg, error) {
	searchResultRequest := internalPb.SearchResult{}
	err := proto.Unmarshal(input, &searchResultRequest)
	if err != nil {
		return nil, err
	}
	searchResultMsg := &SearchResultMsg{SearchResult: searchResultRequest}
	searchResultMsg.BeginTimestamp = searchResultMsg.Timestamp
	searchResultMsg.EndTimestamp = searchResultMsg.Timestamp

	return searchResultMsg, nil
}

/////////////////////////////////////////TimeTick//////////////////////////////////////////
type TimeTickMsg struct {
	BaseMsg
	internalPb.TimeTickMsg
}

func (tst *TimeTickMsg) Type() MsgType {
	return tst.MsgType
}

func (tst *TimeTickMsg) GetMsgContext() context.Context {
	return tst.MsgCtx
}

func (tst *TimeTickMsg) SetMsgContext(ctx context.Context) {
	tst.MsgCtx = ctx
}

func (tst *TimeTickMsg) Marshal(input TsMsg) ([]byte, error) {
	timeTickTask := input.(*TimeTickMsg)
	timeTick := &timeTickTask.TimeTickMsg
	mb, err := proto.Marshal(timeTick)
	if err != nil {
		return nil, err
	}
	return mb, nil
}

func (tst *TimeTickMsg) Unmarshal(input []byte) (TsMsg, error) {
	timeTickMsg := internalPb.TimeTickMsg{}
	err := proto.Unmarshal(input, &timeTickMsg)
	if err != nil {
		return nil, err
	}
	timeTick := &TimeTickMsg{TimeTickMsg: timeTickMsg}
	timeTick.BeginTimestamp = timeTick.Timestamp
	timeTick.EndTimestamp = timeTick.Timestamp

	return timeTick, nil
}

/////////////////////////////////////////QueryNodeStats//////////////////////////////////////////
type QueryNodeStatsMsg struct {
	BaseMsg
	internalPb.QueryNodeStats
}

func (qs *QueryNodeStatsMsg) Type() MsgType {
	return qs.MsgType
}

func (qs *QueryNodeStatsMsg) GetMsgContext() context.Context {
	return qs.MsgCtx
}

func (qs *QueryNodeStatsMsg) SetMsgContext(ctx context.Context) {
	qs.MsgCtx = ctx
}

func (qs *QueryNodeStatsMsg) Marshal(input TsMsg) ([]byte, error) {
	queryNodeSegStatsTask := input.(*QueryNodeStatsMsg)
	queryNodeSegStats := &queryNodeSegStatsTask.QueryNodeStats
	mb, err := proto.Marshal(queryNodeSegStats)
	if err != nil {
		return nil, err
	}
	return mb, nil
}

func (qs *QueryNodeStatsMsg) Unmarshal(input []byte) (TsMsg, error) {
	queryNodeSegStats := internalPb.QueryNodeStats{}
	err := proto.Unmarshal(input, &queryNodeSegStats)
	if err != nil {
		return nil, err
	}
	queryNodeSegStatsMsg := &QueryNodeStatsMsg{QueryNodeStats: queryNodeSegStats}

	return queryNodeSegStatsMsg, nil
}

///////////////////////////////////////////Key2Seg//////////////////////////////////////////
//type Key2SegMsg struct {
//	BaseMsg
//	internalPb.Key2SegMsg
//}
//
//func (k2st *Key2SegMsg) Type() MsgType {
//	return
//}

/////////////////////////////////////////CreateCollection//////////////////////////////////////////
type CreateCollectionMsg struct {
	BaseMsg
	internalPb.CreateCollectionRequest
}

func (cc *CreateCollectionMsg) Type() MsgType {
	return cc.MsgType
}

func (cc *CreateCollectionMsg) GetMsgContext() context.Context {
	return cc.MsgCtx
}

func (cc *CreateCollectionMsg) SetMsgContext(ctx context.Context) {
	cc.MsgCtx = ctx
}

func (cc *CreateCollectionMsg) Marshal(input TsMsg) ([]byte, error) {
	createCollectionMsg := input.(*CreateCollectionMsg)
	createCollectionRequest := &createCollectionMsg.CreateCollectionRequest
	mb, err := proto.Marshal(createCollectionRequest)
	if err != nil {
		return nil, err
	}
	return mb, nil
}

func (cc *CreateCollectionMsg) Unmarshal(input []byte) (TsMsg, error) {
	createCollectionRequest := internalPb.CreateCollectionRequest{}
	err := proto.Unmarshal(input, &createCollectionRequest)
	if err != nil {
		return nil, err
	}
	createCollectionMsg := &CreateCollectionMsg{CreateCollectionRequest: createCollectionRequest}
	createCollectionMsg.BeginTimestamp = createCollectionMsg.Timestamp
	createCollectionMsg.EndTimestamp = createCollectionMsg.Timestamp

	return createCollectionMsg, nil
}

/////////////////////////////////////////DropCollection//////////////////////////////////////////
type DropCollectionMsg struct {
	BaseMsg
	internalPb.DropCollectionRequest
}

func (dc *DropCollectionMsg) Type() MsgType {
	return dc.MsgType
}
func (dc *DropCollectionMsg) GetMsgContext() context.Context {
	return dc.MsgCtx
}

func (dc *DropCollectionMsg) SetMsgContext(ctx context.Context) {
	dc.MsgCtx = ctx
}

func (dc *DropCollectionMsg) Marshal(input TsMsg) ([]byte, error) {
	dropCollectionMsg := input.(*DropCollectionMsg)
	dropCollectionRequest := &dropCollectionMsg.DropCollectionRequest
	mb, err := proto.Marshal(dropCollectionRequest)
	if err != nil {
		return nil, err
	}
	return mb, nil
}

func (dc *DropCollectionMsg) Unmarshal(input []byte) (TsMsg, error) {
	dropCollectionRequest := internalPb.DropCollectionRequest{}
	err := proto.Unmarshal(input, &dropCollectionRequest)
	if err != nil {
		return nil, err
	}
	dropCollectionMsg := &DropCollectionMsg{DropCollectionRequest: dropCollectionRequest}
	dropCollectionMsg.BeginTimestamp = dropCollectionMsg.Timestamp
	dropCollectionMsg.EndTimestamp = dropCollectionMsg.Timestamp

	return dropCollectionMsg, nil
}

/////////////////////////////////////////CreatePartition//////////////////////////////////////////
type CreatePartitionMsg struct {
	BaseMsg
	internalPb.CreatePartitionRequest
}

func (cc *CreatePartitionMsg) GetMsgContext() context.Context {
	return cc.MsgCtx
}

func (cc *CreatePartitionMsg) SetMsgContext(ctx context.Context) {
	cc.MsgCtx = ctx
}

func (cc *CreatePartitionMsg) Type() MsgType {
	return cc.MsgType
}

func (cc *CreatePartitionMsg) Marshal(input TsMsg) ([]byte, error) {
	createPartitionMsg := input.(*CreatePartitionMsg)
	createPartitionRequest := &createPartitionMsg.CreatePartitionRequest
	mb, err := proto.Marshal(createPartitionRequest)
	if err != nil {
		return nil, err
	}
	return mb, nil
}

func (cc *CreatePartitionMsg) Unmarshal(input []byte) (TsMsg, error) {
	createPartitionRequest := internalPb.CreatePartitionRequest{}
	err := proto.Unmarshal(input, &createPartitionRequest)
	if err != nil {
		return nil, err
	}
	createPartitionMsg := &CreatePartitionMsg{CreatePartitionRequest: createPartitionRequest}
	createPartitionMsg.BeginTimestamp = createPartitionMsg.Timestamp
	createPartitionMsg.EndTimestamp = createPartitionMsg.Timestamp

	return createPartitionMsg, nil
}

/////////////////////////////////////////DropPartition//////////////////////////////////////////
type DropPartitionMsg struct {
	BaseMsg
	internalPb.DropPartitionRequest
}

func (dc *DropPartitionMsg) GetMsgContext() context.Context {
	return dc.MsgCtx
}

func (dc *DropPartitionMsg) SetMsgContext(ctx context.Context) {
	dc.MsgCtx = ctx
}

func (dc *DropPartitionMsg) Type() MsgType {
	return dc.MsgType
}

func (dc *DropPartitionMsg) Marshal(input TsMsg) ([]byte, error) {
	dropPartitionMsg := input.(*DropPartitionMsg)
	dropPartitionRequest := &dropPartitionMsg.DropPartitionRequest
	mb, err := proto.Marshal(dropPartitionRequest)
	if err != nil {
		return nil, err
	}
	return mb, nil
}

func (dc *DropPartitionMsg) Unmarshal(input []byte) (TsMsg, error) {
	dropPartitionRequest := internalPb.DropPartitionRequest{}
	err := proto.Unmarshal(input, &dropPartitionRequest)
	if err != nil {
		return nil, err
	}
	dropPartitionMsg := &DropPartitionMsg{DropPartitionRequest: dropPartitionRequest}
	dropPartitionMsg.BeginTimestamp = dropPartitionMsg.Timestamp
	dropPartitionMsg.EndTimestamp = dropPartitionMsg.Timestamp

	return dropPartitionMsg, nil
}

/////////////////////////////////////////LoadIndex//////////////////////////////////////////
type LoadIndexMsg struct {
	BaseMsg
	internalPb.LoadIndex
}

func (lim *LoadIndexMsg) Type() MsgType {
	return lim.MsgType
}

func (lim *LoadIndexMsg) GetMsgContext() context.Context {
	return lim.MsgCtx
}

func (lim *LoadIndexMsg) SetMsgContext(ctx context.Context) {
	lim.MsgCtx = ctx
}

func (lim *LoadIndexMsg) Marshal(input TsMsg) ([]byte, error) {
	loadIndexMsg := input.(*LoadIndexMsg)
	loadIndexRequest := &loadIndexMsg.LoadIndex
	mb, err := proto.Marshal(loadIndexRequest)
	if err != nil {
		return nil, err
	}
	return mb, nil
}

func (lim *LoadIndexMsg) Unmarshal(input []byte) (TsMsg, error) {
	loadIndexRequest := internalPb.LoadIndex{}
	err := proto.Unmarshal(input, &loadIndexRequest)
	if err != nil {
		return nil, err
	}
	loadIndexMsg := &LoadIndexMsg{LoadIndex: loadIndexRequest}

	return loadIndexMsg, nil
}
