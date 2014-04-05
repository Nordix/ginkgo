/*

Aggregator is a reporter used by the Ginkgo CLI to aggregate and present parallel test output
coherently as tests complete.  You shouldn't need to use this in your code.  To run tests in parallel:

	ginkgo -nodes=N

where N is the number of nodes you desire.
*/
package remote

import (
	"github.com/onsi/ginkgo/config"
	"github.com/onsi/ginkgo/reporters/stenographer"
	"github.com/onsi/ginkgo/types"
	"time"
)

type configAndSuite struct {
	config  config.GinkgoConfigType
	summary *types.SuiteSummary
}

type Aggregator struct {
	nodeCount    int
	config       config.DefaultReporterConfigType
	stenographer stenographer.Stenographer
	result       chan bool

	suiteBeginnings           chan configAndSuite
	aggregatedSuiteBeginnings []configAndSuite

	beforeSuites           chan *types.SetupSummary
	aggregatedBeforeSuites []*types.SetupSummary

	specCompletions chan *types.SpecSummary
	completedSpecs  []*types.SpecSummary

	suiteEndings           chan *types.SuiteSummary
	aggregatedSuiteEndings []*types.SuiteSummary

	startTime time.Time
}

func NewAggregator(nodeCount int, result chan bool, config config.DefaultReporterConfigType, stenographer stenographer.Stenographer) *Aggregator {
	aggregator := &Aggregator{
		nodeCount:    nodeCount,
		result:       result,
		config:       config,
		stenographer: stenographer,

		suiteBeginnings:           make(chan configAndSuite, 0),
		aggregatedSuiteBeginnings: []configAndSuite{},

		beforeSuites: make(chan *types.SetupSummary, 0),

		specCompletions: make(chan *types.SpecSummary, 0),
		completedSpecs:  []*types.SpecSummary{},

		suiteEndings:           make(chan *types.SuiteSummary, 0),
		aggregatedSuiteEndings: []*types.SuiteSummary{},
	}

	go aggregator.mux()

	return aggregator
}

func (aggregator *Aggregator) SpecSuiteWillBegin(config config.GinkgoConfigType, summary *types.SuiteSummary) {
	aggregator.suiteBeginnings <- configAndSuite{config, summary}
}

func (aggregator *Aggregator) BeforeSuiteDidRun(setupSummary *types.SetupSummary) {
	aggregator.beforeSuites <- setupSummary
}

func (aggregator *Aggregator) SpecWillRun(specSummary *types.SpecSummary) {
	//noop
}

func (aggregator *Aggregator) SpecDidComplete(specSummary *types.SpecSummary) {
	aggregator.specCompletions <- specSummary
}

func (aggregator *Aggregator) SpecSuiteDidEnd(summary *types.SuiteSummary) {
	aggregator.suiteEndings <- summary
}

func (aggregator *Aggregator) mux() {
loop:
	for {
		select {
		case configAndSuite := <-aggregator.suiteBeginnings:
			aggregator.registerSuiteBeginning(configAndSuite)
		case setupSummary := <-aggregator.beforeSuites:
			aggregator.registerBeforeSuite(setupSummary)
		case specSummary := <-aggregator.specCompletions:
			aggregator.registerSpecCompletion(specSummary)
		case suite := <-aggregator.suiteEndings:
			finished, passed := aggregator.registerSuiteEnding(suite)
			if finished {
				aggregator.result <- passed
				break loop
			}
		}
	}
}

func (aggregator *Aggregator) registerSuiteBeginning(configAndSuite configAndSuite) {
	aggregator.aggregatedSuiteBeginnings = append(aggregator.aggregatedSuiteBeginnings, configAndSuite)

	if len(aggregator.aggregatedSuiteBeginnings) == 1 {
		aggregator.startTime = time.Now()
	}

	if len(aggregator.aggregatedSuiteBeginnings) != aggregator.nodeCount {
		return
	}

	aggregator.stenographer.AnnounceSuite(configAndSuite.summary.SuiteDescription, configAndSuite.config.RandomSeed, configAndSuite.config.RandomizeAllSpecs, aggregator.config.Succinct)

	numberOfSpecsToRun := 0
	totalNumberOfSpecs := 0
	for _, configAndSuite := range aggregator.aggregatedSuiteBeginnings {
		numberOfSpecsToRun += configAndSuite.summary.NumberOfSpecsThatWillBeRun
		totalNumberOfSpecs += configAndSuite.summary.NumberOfTotalSpecs
	}

	aggregator.stenographer.AnnounceNumberOfSpecs(numberOfSpecsToRun, totalNumberOfSpecs, aggregator.config.Succinct)
	aggregator.stenographer.AnnounceAggregatedParallelRun(aggregator.nodeCount, aggregator.config.Succinct)
	aggregator.flushCompletedSpecs()
}

func (aggregator *Aggregator) registerBeforeSuite(setupSummary *types.SetupSummary) {
	aggregator.aggregatedBeforeSuites = append(aggregator.aggregatedBeforeSuites, setupSummary)
	aggregator.flushCompletedSpecs()
}

func (aggregator *Aggregator) registerSpecCompletion(specSummary *types.SpecSummary) {
	aggregator.completedSpecs = append(aggregator.completedSpecs, specSummary)
	aggregator.flushCompletedSpecs()
}

func (aggregator *Aggregator) flushCompletedSpecs() {
	if len(aggregator.aggregatedSuiteBeginnings) != aggregator.nodeCount {
		return
	}

	for _, setupSummary := range aggregator.aggregatedBeforeSuites {
		aggregator.announceBeforeSuite(setupSummary)
	}

	for _, specSummary := range aggregator.completedSpecs {
		aggregator.announceSpec(specSummary)
	}

	aggregator.aggregatedBeforeSuites = []*types.SetupSummary{}
	aggregator.completedSpecs = []*types.SpecSummary{}
}

func (aggregator *Aggregator) announceBeforeSuite(setupSummary *types.SetupSummary) {
	aggregator.stenographer.AnnounceCapturedOutput(setupSummary.CapturedOutput)
	if setupSummary.State != types.SpecStatePassed {
		aggregator.stenographer.AnnounceBeforeSuiteFailure(setupSummary, aggregator.config.Succinct)
	}
}

func (aggregator *Aggregator) announceSpec(specSummary *types.SpecSummary) {
	if aggregator.config.Verbose && specSummary.State != types.SpecStatePending && specSummary.State != types.SpecStateSkipped {
		aggregator.stenographer.AnnounceSpecWillRun(specSummary)
	}

	aggregator.stenographer.AnnounceCapturedOutput(specSummary.CapturedOutput)

	switch specSummary.State {
	case types.SpecStatePassed:
		if specSummary.IsMeasurement {
			aggregator.stenographer.AnnounceSuccesfulMeasurement(specSummary, aggregator.config.Succinct)
		} else if specSummary.RunTime.Seconds() >= aggregator.config.SlowSpecThreshold {
			aggregator.stenographer.AnnounceSuccesfulSlowSpec(specSummary, aggregator.config.Succinct)
		} else {
			aggregator.stenographer.AnnounceSuccesfulSpec(specSummary)
		}

	case types.SpecStatePending:
		aggregator.stenographer.AnnouncePendingSpec(specSummary, aggregator.config.NoisyPendings && !aggregator.config.Succinct)
	case types.SpecStateSkipped:
		aggregator.stenographer.AnnounceSkippedSpec(specSummary)
	case types.SpecStateTimedOut:
		aggregator.stenographer.AnnounceSpecTimedOut(specSummary, aggregator.config.Succinct)
	case types.SpecStatePanicked:
		aggregator.stenographer.AnnounceSpecPanicked(specSummary, aggregator.config.Succinct)
	case types.SpecStateFailed:
		aggregator.stenographer.AnnounceSpecFailed(specSummary, aggregator.config.Succinct)
	}
}

func (aggregator *Aggregator) registerSuiteEnding(suite *types.SuiteSummary) (finished bool, passed bool) {
	aggregator.aggregatedSuiteEndings = append(aggregator.aggregatedSuiteEndings, suite)
	if len(aggregator.aggregatedSuiteEndings) < aggregator.nodeCount {
		return false, false
	}

	aggregatedSuiteSummary := &types.SuiteSummary{}
	aggregatedSuiteSummary.SuiteSucceeded = true

	for _, suiteSummary := range aggregator.aggregatedSuiteEndings {
		if suiteSummary.SuiteSucceeded == false {
			aggregatedSuiteSummary.SuiteSucceeded = false
		}

		aggregatedSuiteSummary.NumberOfSpecsThatWillBeRun += suiteSummary.NumberOfSpecsThatWillBeRun
		aggregatedSuiteSummary.NumberOfTotalSpecs += suiteSummary.NumberOfTotalSpecs
		aggregatedSuiteSummary.NumberOfPassedSpecs += suiteSummary.NumberOfPassedSpecs
		aggregatedSuiteSummary.NumberOfFailedSpecs += suiteSummary.NumberOfFailedSpecs
		aggregatedSuiteSummary.NumberOfPendingSpecs += suiteSummary.NumberOfPendingSpecs
		aggregatedSuiteSummary.NumberOfSkippedSpecs += suiteSummary.NumberOfSkippedSpecs
	}

	aggregatedSuiteSummary.RunTime = time.Since(aggregator.startTime)
	aggregator.stenographer.AnnounceSpecRunCompletion(aggregatedSuiteSummary, aggregator.config.Succinct)

	return true, aggregatedSuiteSummary.SuiteSucceeded
}