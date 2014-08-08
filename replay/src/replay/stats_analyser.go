package replay

import (
	"sort"
	"time"
)

var (
	latencyPercentiles = []int{50, 60, 70, 80, 90, 95, 99, 100}
	emptyLatencies     = make([]int64, len(latencyPercentiles))
)

// Percentiles
const (
	P50  = iota
	P60  = iota
	P70  = iota
	P80  = iota
	P90  = iota
	P95  = iota
	P99  = iota
	P100 = iota
)

func NewStatsAnalyzer(
	statsCollectors []*StatsCollector,
	opsExecuted *int64,
	latencyChan chan Latency,
	latenciesSize int) *StatsAnalyzer {
	latencies := map[OpType][]int64{}
	lastEndPos := map[OpType]int{}
	countsLast := make(map[OpType]int64)

	for _, opType := range AllOpTypes {
		latencies[opType] = make([]int64, 0, latenciesSize)
		lastEndPos[opType] = 0
	}

	go func() {
		for {
			op, ok := <-latencyChan
			if !ok {
				break
			}
			latencies[op.OpType] = append(
				latencies[op.OpType], int64(op.Latency),
			)
		}
	}()

	return &StatsAnalyzer{
		statsCollectors: statsCollectors,
		opsExecuted:     opsExecuted,
		opsExecutedLast: 0,
		latencyChan:     latencyChan,
		latencies:       latencies,
		epoch:           time.Now(),
		timeLast:	 time.Now(),
		lastEndPos:      lastEndPos,
		countsLast:	 countsLast,
	}
}

// ExecutionStatus encapsulates the aggregated information for the execution
type ExecutionStatus struct {
	OpsExecuted        int64
	// OpsPerSec stores ops/sec averaged across the entire workload
	OpsPerSec          float64
	// OpsPerSecLast stores the ops/sec since the last call to GetStatus()
	OpsPerSecLast	   float64
	Duration           time.Duration
	AllTimeLatencies   map[OpType][]int64
	SinceLastLatencies map[OpType][]int64
	Counts             map[OpType]int64
	TypeOpsSec         map[OpType]float64
	TypeOpsSecLast     map[OpType]float64
}

type StatsAnalyzer struct {
	statsCollectors []*StatsCollector
	// store total ops executed during the run
	opsExecuted     *int64
	// store ops executed at the time of the last GetStatus() call
	opsExecutedLast int64
	latencyChan     chan Latency
	latencies       map[OpType][]int64
	// Store the start of the run
	epoch           time.Time
	// Store the time of the last GetStatus() call
	timeLast	time.Time
	lastEndPos      map[OpType]int
	countsLast	map[OpType]int64
}

func (self *StatsAnalyzer) GetStatus() *ExecutionStatus {
	// Basics
	duration := time.Now().Sub(self.epoch)
	opsPerSec := 0.0
	if duration != 0 {
		opsPerSec = float64(*self.opsExecuted) * float64(time.Second) / float64(duration)
	}
	// Calculate ops/sec since last call to GetStatus()
	lastDuration := time.Now().Sub(self.timeLast)
	opsPerSecLast := 0.0
	if lastDuration != 0 {
		opsPerSecLast = float64(*self.opsExecuted - self.opsExecutedLast) * float64(time.Second) / float64(lastDuration)
	}
	
	self.opsExecutedLast = *self.opsExecuted
	self.timeLast = time.Now()

	// Latencies
	stats := CombineStats(self.statsCollectors...)
	allTimeLatencies := make(map[OpType][]int64)
	sinceLastLatencies := make(map[OpType][]int64)
	counts := make(map[OpType]int64)
	typeOpsSec := make(map[OpType]float64)
	typeOpsSecLast := make(map[OpType]float64)

	for _, opType := range AllOpTypes {
		// take a snapshot of current status since the latency list keeps
		// increasing.
		length := len(self.latencies[opType])
		snapshot := self.latencies[opType][:length]
		lastEndPos := self.lastEndPos[opType]
		self.lastEndPos[opType] = length
		sinceLastLatencies[opType] =
			CalculateLatencyStats(snapshot[lastEndPos:])
		allTimeLatencies[opType] = CalculateLatencyStats(snapshot)
		counts[opType] = stats.Count(opType)
		
		typeOpsSec[opType] = 0.0
		typeOpsSecLast[opType] = 0.0
		if duration != 0 {
			typeOpsSec[opType] = float64(counts[opType]) * float64(time.Second) / float64(duration)
		}
		if lastDuration != 0 {
			typeOpsSecLast[opType] = float64(counts[opType] - self.countsLast[opType]) * float64(time.Second) / float64(lastDuration)
		}
		self.countsLast[opType] = counts[opType]
	}

	status := ExecutionStatus{
		OpsExecuted:        *self.opsExecuted,
		Duration:           duration,
		OpsPerSec:          opsPerSec,
		OpsPerSecLast:	    opsPerSecLast,
		AllTimeLatencies:   allTimeLatencies,
		SinceLastLatencies: sinceLastLatencies,
		Counts:             counts,
		TypeOpsSec:         typeOpsSec,
		TypeOpsSecLast:         typeOpsSecLast,
	}

	return &status
}

// Sorting facilities
type int64Slice []int64

func (p int64Slice) Len() int           { return len(p) }
func (p int64Slice) Less(i, j int) bool { return p[i] < p[j] }
func (p int64Slice) Swap(i, j int)      { p[i], p[j] = p[j], p[i] }

func CalculateLatencyStats(latencies []int64) []int64 {
	result := make([]int64, 0, len(latencyPercentiles))
	length := len(latencies)
	if length == 0 {
		return emptyLatencies
	}
	sort.Sort(int64Slice(latencies))
	for _, perc := range latencyPercentiles {
		result = append(result, latencies[(length-1)*perc/100])
	}
	return result
}
