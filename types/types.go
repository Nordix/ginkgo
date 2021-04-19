package types

import (
	"strings"
	"time"
)

const GINKGO_FOCUS_EXIT_CODE = 197

// TODO - document Report
type Report struct {
	SuitePath                 string
	SuiteDescription          string
	SuiteSucceeded            bool
	SpecialSuiteFailureReason string

	PreRunStats PreRunStats

	StartTime time.Time
	EndTime   time.Time
	RunTime   time.Duration

	SuiteConfig SuiteConfig
	SpecReports SpecReports
}

type PreRunStats struct {
	TotalSpecs       int
	SpecsThatWillRun int
}

func (report Report) Add(other Report) Report {
	report.SuiteSucceeded = report.SuiteSucceeded && other.SuiteSucceeded

	if other.StartTime.Before(report.StartTime) {
		report.StartTime = other.StartTime
	}

	if other.EndTime.After(report.EndTime) {
		report.EndTime = other.EndTime
	}

	if other.SpecialSuiteFailureReason != "" && report.SpecialSuiteFailureReason == "" {
		report.SpecialSuiteFailureReason = other.SpecialSuiteFailureReason
	}

	report.RunTime = report.EndTime.Sub(report.StartTime)

	reports := make(SpecReports, len(report.SpecReports)+len(other.SpecReports))
	for i := range report.SpecReports {
		reports[i] = report.SpecReports[i]
	}
	offset := len(report.SpecReports)
	for i := range other.SpecReports {
		reports[i+offset] = other.SpecReports[i]
	}

	report.SpecReports = reports
	return report
}

// SpecReport captures information about a Ginkgo spec.
type SpecReport struct {
	// ContainerHierarchyTexts is a slice containing the text strings of
	// all Describe/Context/When containers in this spec's hierarchy.
	ContainerHierarchyTexts []string

	// ContainerHierarchyLocations is a slice containing the CodeLocations of
	// all Describe/Context/When containers in this spec's hirerachy.
	ContainerHierarchyLocations []CodeLocation

	// LeafNodeType, LeadNodeLocation, and LeafNodeText capture the NodeType, CodeLocation, and text
	// of the Ginkgo node being tested (typically an NodeTypeIt node, though this can also be
	// one of the NodeTypesForSuiteLevelNodes node types)
	LeafNodeType     NodeType
	LeafNodeLocation CodeLocation
	LeafNodeText     string

	// State captures whether the spec has passed, failed, etc.
	State SpecState

	// StartTime and Entime capture the start and end time of the spec
	StartTime time.Time
	EndTime   time.Time

	// RunTime captures the duration of the spec
	RunTime time.Duration

	// GinkgoParallelNode captures the parallel node that this spec ran on
	GinkgoParallelNode int

	//Failure is populated if a spec has failed, panicked, been interrupted, or skipped by the user (e.g. calling Skip())
	//It includes detailed information about the Failure
	Failure Failure

	// NumAttempts captures the number of times this Spec was run.  Flakey specs can be retried with
	// ginkgo --flake-attempts=N
	NumAttempts int

	// CapturedGinkgoWriterOutput contains text printed to the GinkgoWriter
	CapturedGinkgoWriterOutput string

	// CapturedStdOutErr contains text printed to stdout/stderr (when running in parallel)
	// This is always empty when running in series or calling CurrentSpecReport()
	// It is used internally by Ginkgo's reporter
	CapturedStdOutErr string
}

// CombinedOutput returns a single string representation of both CapturedStdOutErr and CapturedGinkgoWriterOutput
// Note that both are empty when using CurrentSpecReport() so CurrentSpecReport().CombinedOutput() will always be empty.
// CombinedOutput() is used internally by Ginkgo's reporter.
func (report SpecReport) CombinedOutput() string {
	if report.CapturedStdOutErr == "" {
		return report.CapturedGinkgoWriterOutput
	}
	if report.CapturedGinkgoWriterOutput == "" {
		return report.CapturedStdOutErr
	}
	return report.CapturedStdOutErr + "\n" + report.CapturedGinkgoWriterOutput
}

//Failed returns true if report.State is one of the SpecStateFailureStates
// (SpecStateFAiled, SpecStatePanicked, SpecStateinterrupted)
func (report SpecReport) Failed() bool {
	return report.State.Is(SpecStateFailureStates...)
}

//FullText returns a concatenation of all the report.NodeTexts
func (report SpecReport) FullText() string {
	texts := []string{}
	texts = append(texts, report.ContainerHierarchyTexts...)
	if report.LeafNodeText != "" {
		texts = append(texts, report.LeafNodeText)
	}
	return strings.Join(texts, " ")
}

//FileName() returns the name of the file containing the spec
func (report SpecReport) FileName() string {
	return report.LeafNodeLocation.FileName
}

//LineNumber() returns the line number of the leaf node
func (report SpecReport) LineNumber() int {
	return report.LeafNodeLocation.LineNumber
}

//FailureMessage() returns the failure message (or empty string if the test hasn't failed)
func (report SpecReport) FailureMessage() string {
	return report.Failure.Message
}

//FailureLocation() returns the location of the failure (or an empty CodeLocation if the test hasn't failed)
func (report SpecReport) FailureLocation() CodeLocation {
	return report.Failure.Location
}

type SpecReports []SpecReport

func (reports SpecReports) WithLeafNodeType(nodeType ...NodeType) SpecReports {
	out := SpecReports{}
	for _, report := range reports {
		if report.LeafNodeType.Is(nodeType...) {
			out = append(out, report)
		}
	}
	return out
}

func (reports SpecReports) WithState(states ...SpecState) SpecReports {
	out := SpecReports{}
	for _, report := range reports {
		if report.State.Is(states...) {
			out = append(out, report)
		}
	}
	return out
}

func (reports SpecReports) CountWithState(states ...SpecState) int {
	n := 0
	for _, report := range reports {
		if report.State.Is(states...) {
			n += 1
		}
	}
	return n
}

func (reports SpecReports) CountOfFlakedSpecs() int {
	n := 0
	for _, report := range reports.WithState(SpecStatePassed) {
		if report.NumAttempts > 1 {
			n += 1
		}
	}
	return n
}

type FailureNodeContext uint

const (
	FailureNodeContextInvalid FailureNodeContext = iota

	FailureNodeIsLeafNode
	FailureNodeAtTopLevel
	FailureNodeInContainer
)

// Failure captures failure information for an individual test
type Failure struct {
	// Message - the failure message passed into Fail(...).  When using a matcher library
	// like Gomega, this will contain the failure message generated by Gomega.
	Message string

	// Location - the CodeLocation where the failure occurred
	// This CodeLocation will include a fully-populated StackTrace
	Location CodeLocation

	// ForwardedPanic - if the failure represents a captured panic (i.e. Summary.State == SpecStatePanicked)
	// then ForwardedPanic will be populated with a string representation of the captured panic.
	ForwardedPanic string

	// FailureNodeContext - one of three contexts describing the node in which the failure occured:
	// FailureNodeIsLeafNode means the failure occured in the leaf node of the associated SpecReport. None of the other FailureNode fields will be populated
	// FailureNodeAtTopLevel means the failure occured in a non-leaf node that is defined at the top-level of the spec (i.e. not in a container). FailureNodeType and FailureNodeLocation will be populated.
	// FailureNodeInContainer means the failure occured in a non-leaf node that is defined within a container.  FailureNodeType, FailureNodeLocaiton, and FailureNodeContainerIndex will be populated.
	//
	//  FailureNodeType will contain the NodeType of the node in which the failure occurred.
	//  FailureNodeLocation will contain the CodeLocation of the node in which the failure occurred.
	// If populated, FailureNodeContainerIndex will be the index into SpecReport.ContainerHierarchyTexts and SpecReport.ContainerHierarchyLocations that represents the parent container of the node in which the failure occurred.
	FailureNodeContext        FailureNodeContext
	FailureNodeType           NodeType
	FailureNodeLocation       CodeLocation
	FailureNodeContainerIndex int
}

func (f Failure) IsZero() bool {
	return f == Failure{}
}

type SpecState uint

const (
	SpecStateInvalid SpecState = iota

	SpecStatePending
	SpecStateSkipped
	SpecStatePassed
	SpecStateFailed
	SpecStatePanicked
	SpecStateInterrupted
)

func (s SpecState) String() string {
	switch s {
	case SpecStatePending:
		return "pending"
	case SpecStateSkipped:
		return "skipped"
	case SpecStatePassed:
		return "passed"
	case SpecStateFailed:
		return "failed"
	case SpecStatePanicked:
		return "panicked"
	case SpecStateInterrupted:
		return "interrupted"
	}

	return "INVALID SPEC STATE"
}

var SpecStateFailureStates = []SpecState{SpecStateFailed, SpecStatePanicked, SpecStateInterrupted}

func (state SpecState) Is(states ...SpecState) bool {
	for _, testState := range states {
		if testState == state {
			return true
		}
	}

	return false
}

type NodeType uint

const (
	NodeTypeInvalid NodeType = iota

	NodeTypeContainer
	NodeTypeIt

	NodeTypeBeforeEach
	NodeTypeJustBeforeEach
	NodeTypeAfterEach
	NodeTypeJustAfterEach

	NodeTypeBeforeSuite
	NodeTypeSynchronizedBeforeSuite
	NodeTypeAfterSuite
	NodeTypeSynchronizedAfterSuite

	NodeTypeReportAfterEach
	NodeTypeReportAfterSuite
)

var NodeTypesForContainerAndIt = []NodeType{NodeTypeContainer, NodeTypeIt}
var NodeTypesForSuiteLevelNodes = []NodeType{NodeTypeBeforeSuite, NodeTypeSynchronizedBeforeSuite, NodeTypeAfterSuite, NodeTypeSynchronizedAfterSuite, NodeTypeReportAfterSuite}

func (nt NodeType) Is(nodeTypes ...NodeType) bool {
	for _, nodeType := range nodeTypes {
		if nt == nodeType {
			return true
		}
	}

	return false
}

func (nt NodeType) String() string {
	switch nt {
	case NodeTypeContainer:
		return "Container"
	case NodeTypeIt:
		return "It"
	case NodeTypeBeforeEach:
		return "BeforeEach"
	case NodeTypeJustBeforeEach:
		return "JustBeforeEach"
	case NodeTypeAfterEach:
		return "AfterEach"
	case NodeTypeJustAfterEach:
		return "JustAfterEach"
	case NodeTypeBeforeSuite:
		return "BeforeSuite"
	case NodeTypeSynchronizedBeforeSuite:
		return "SynchronizedBeforeSuite"
	case NodeTypeAfterSuite:
		return "AfterSuite"
	case NodeTypeSynchronizedAfterSuite:
		return "SynchronizedAfterSuite"
	case NodeTypeReportAfterEach:
		return "ReportAfterEach"
	case NodeTypeReportAfterSuite:
		return "ReportAfterSuite"
	}

	return "INVALID NODE TYPE"
}
