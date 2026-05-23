package rustygo_test

import (
	"bytes"
	rg "rustygo"
	"testing"
)

type requestRecord struct {
	Method       [8]byte
	PathLen      int
	Status       int
	SegmentCount int
	QueryPairs   int
	DurationUS   int
}

type requestSegment struct {
	Start int
	End   int
}

var (
	requestRecordSink  *requestRecord
	requestSegmentSink []requestSegment
	requestBatchLines  = [][]byte{
		[]byte("GET /v1/users/42/profile?expand=teams&verbose=1 200 187"),
		[]byte("POST /v1/orders/918/items?region=eu&priority=high 201 244"),
		[]byte("PUT /v1/inventory/sku-77?warehouse=west&sync=1 202 311"),
		[]byte("GET /v1/search?q=arena+allocator&limit=25&page=2 200 129"),
		[]byte("DELETE /v1/sessions/a81f2c?hard=true 204 91"),
		[]byte("PATCH /v1/accounts/14/preferences?theme=light&tz=utc 200 163"),
		[]byte("GET /v1/metrics/runtime?window=5m&group=service 200 141"),
		[]byte("POST /v1/events/ingest?source=edge&compress=gzip 202 278"),
	}
)

//go:noinline
func consumeRequestRecord(v *requestRecord) {
	requestRecordSink = v
}

//go:noinline
func consumeRequestSegments(v []requestSegment) {
	requestSegmentSink = v
}

func BenchmarkRequestBatchArenaVsHeap(b *testing.B) {
	batchBytes := 0
	for _, line := range requestBatchLines {
		batchBytes += len(line)
	}
	b.SetBytes(int64(batchBytes))

	b.Run("Heap", func(b *testing.B) {
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			sinkInt ^= int64(processRequestBatchHeap())
		}
	})

	b.Run("Arena", func(b *testing.B) {
		arena := rg.NewArena(2 * 1024 * 1024)
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			scope := arena.EnterScope()
			sinkInt ^= int64(processRequestBatchArena(scope))
			scope.Exit()
			arena.Reset()
		}
	})
}

func processRequestBatchHeap() int {
	total := 0
	for _, line := range requestBatchLines {
		rec := new(requestRecord)
		pathStart, pathEnd, status, duration := parseRequestLine(line, rec)
		pathCopy := make([]byte, pathEnd-pathStart)
		copy(pathCopy, line[pathStart:pathEnd])
		segments := make([]requestSegment, 0, 8)
		segments = collectSegments(pathCopy, segments)
		rec.SegmentCount = len(segments)
		rec.QueryPairs = bytes.Count(pathCopy, []byte{'&'})
		if bytes.IndexByte(pathCopy, '?') >= 0 {
			rec.QueryPairs++
		}
		rec.Status = status
		rec.DurationUS = duration
		rec.PathLen = len(pathCopy)
		total += rec.Status + rec.DurationUS + rec.PathLen + rec.SegmentCount + rec.QueryPairs
		consumeBuf(pathCopy)
		consumeRequestSegments(segments)
		consumeRequestRecord(rec)
	}
	return total
}

func processRequestBatchArena(scope *rg.Scope) int {
	total := 0
	for _, line := range requestBatchLines {
		rec := rg.AllocValue[requestRecord](scope)
		*rec = requestRecord{}
		pathStart, pathEnd, status, duration := parseRequestLine(line, rec)
		pathCopy := rg.AllocSlice[byte](scope, pathEnd-pathStart)
		copy(pathCopy, line[pathStart:pathEnd])
		segments := rg.AllocSliceCap[requestSegment](scope, 0, 8)
		segments = collectSegments(pathCopy, segments)
		rec.SegmentCount = len(segments)
		rec.QueryPairs = bytes.Count(pathCopy, []byte{'&'})
		if bytes.IndexByte(pathCopy, '?') >= 0 {
			rec.QueryPairs++
		}
		rec.Status = status
		rec.DurationUS = duration
		rec.PathLen = len(pathCopy)
		total += rec.Status + rec.DurationUS + rec.PathLen + rec.SegmentCount + rec.QueryPairs
		consumeBuf(pathCopy)
		consumeRequestSegments(segments)
		consumeRequestRecord(rec)
	}
	return total
}

func parseRequestLine(line []byte, rec *requestRecord) (pathStart, pathEnd, status, duration int) {
	space1 := bytes.IndexByte(line, ' ')
	space2 := bytes.IndexByte(line[space1+1:], ' ')
	space2 += space1 + 1
	space3 := bytes.IndexByte(line[space2+1:], ' ')
	space3 += space2 + 1

	method := line[:space1]
	copy(rec.Method[:], method)

	pathStart = space1 + 1
	pathEnd = space2
	status = parseUintASCII(line[space2+1 : space3])
	duration = parseUintASCII(line[space3+1:])
	return pathStart, pathEnd, status, duration
}

func collectSegments(path []byte, segments []requestSegment) []requestSegment {
	start := 0
	for start < len(path) {
		for start < len(path) && (path[start] == '/' || path[start] == '?' || path[start] == '&') {
			start++
		}
		if start >= len(path) {
			break
		}
		end := start
		for end < len(path) && path[end] != '/' && path[end] != '?' && path[end] != '&' {
			end++
		}
		segments = append(segments, requestSegment{Start: start, End: end})
		start = end
	}
	return segments
}

func parseUintASCII(raw []byte) int {
	n := 0
	for _, ch := range raw {
		if ch < '0' || ch > '9' {
			break
		}
		n = n*10 + int(ch-'0')
	}
	return n
}
