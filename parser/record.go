// Copyright 2019-2020 Dolthub, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package parser

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
)

type SortMode string

const (
	NoSort    SortMode = "nosort"
	Rowsort   SortMode = "rowsort"
	ValueSort SortMode = "valuesort"
)

type RecordType int

const (
	// Statement is a record to execute with no results to validate, such as create or insert
	Statement RecordType = iota
	// Query is a record to execute and validate that results are as expected
	Query
	// Halt is a record that terminates the current test script's execution
	Halt
)

// A test script contains many Records, which can be either statements to execute or queries with results.
type Record struct {
	// The type of this record
	recordType RecordType
	// Whether this record expects an error to occur on execution.
	expectError bool
	// The conditions for executing this record, if applicable
	conditions []*Condition
	// The schema for results of this query record, in the form e.g. "ITTR"
	schema string
	// The sort mode for validating results of a query
	sortMode SortMode
	// The query string or statement to execute
	query string
	// The canonical line number for this record, which is the first line number of the SQL statement or
	// query to execute.
	lineNum int
	// The expected result of the query, represented as strings
	result []string
	// Label used to store results for a query, currently unused.
	label string
	// Hash threshold is the number of records to begin hashing results at
	hashThreshold int
}

// A condition is a directive to execute a record or not depending on the underlying engine being evaluated.
type Condition struct {
	isOnly bool
	isSkip bool
	engine string
}

var hashRegex = regexp.MustCompile("(\\d+) values hashing to ([0-9a-f]+)")

// Type returns the type of this record.
func (r *Record) Type() RecordType {
	return r.recordType
}

// ExpectError returns whether this record expects an error to occur on execution.
func (r *Record) ExpectError() bool {
	return r.expectError
}

// Schema returns the schema for the results of this query, in the form e.g. "ITTR"
func (r *Record) Schema() string {
	return r.schema
}

// Query returns the query for this record, which is either a statement to execute or a query to validate results for.
func (r *Record) Query() string {
	return r.query
}

// Returns the expected results of the query for this record. For many records, this is a hash of sorted results
// instead of the full list of values. Use IsHashResult to disambiguate.
func (r *Record) Result() []string {
	return r.result
}

// IsHashResult returns whether this record has a hash result (as opposed to enumerating each value).
func (r *Record) IsHashResult() bool {
	return len(r.result) == 1 && hashRegex.MatchString(r.result[0])
}

// HashResult returns the hash for result values for this record.
func (r *Record) HashResult() string {
	return hashRegex.ReplaceAllString(r.result[0], "$2")
}

// NumRows returns the number of results (not rows) for this record. Panics if the record is a statement instead of a
// query.
func (r *Record) NumResults() int {
	if r.recordType != Query {
		panic("Only query records have results")
	}

	numVals := len(r.result)
	if r.IsHashResult() {
		valsStr := hashRegex.ReplaceAllString(r.result[0], "$1")
		numVals, _ = strconv.Atoi(valsStr)
	}

	return numVals
}

// NumCols returns the number of columns for results of this record's query. Panics if the record is a statement instead
// of a query.
func (r *Record) NumCols() int {
	if r.recordType != Query {
		panic("Only query records have results")
	}

	return len(r.schema)
}

// LineNum returns the canonical line number for this record, which is the first line number of the SQL statement or
// query to execute, excluding any comment lines and conditions.
func (r *Record) LineNum() int {
	return r.lineNum
}

// ShouldExecuteForEngine returns whether this record should be executed for the engine with the identifier given.
func (r *Record) ShouldExecuteForEngine(engine string) bool {
	// skipif and onlyif don't really play nicely together. We honor an onlyif only as the single condition for a record.
	if len(r.conditions) == 1 && r.conditions[0].isOnly {
		return r.conditions[0].engine == engine
	}

	for _, condition := range r.conditions {
		if condition.isSkip && condition.engine == engine {
			return false
		}
	}

	return true
}

// rowSorter sorts a slice of result values with by-row semantics.
type rowSorter struct {
	record *Record
	values []string
}

func (s rowSorter) toRow(i int) []string {
	return s.values[i*s.record.NumCols() : (i+1)*s.record.NumCols()]
}

func (s rowSorter) Len() int {
	return len(s.values) / s.record.NumCols()
}

func (s rowSorter) Less(i, j int) bool {
	rowI := s.toRow(i)
	rowJ := s.toRow(j)
	for k := range rowI {
		if rowI[k] < rowJ[k] {
			return true
		}
		if rowI[k] > rowJ[k] {
			return false
		}
	}
	return false
}

func (s rowSorter) Swap(i, j int) {
	rowI := s.toRow(i)
	rowJ := s.toRow(j)
	for col := range rowI {
		rowI[col], rowJ[col] = rowJ[col], rowI[col]
	}
}

// Sort results sorts the input slice (the results of this record's query) according to the record's specification
// (no sorting, row-based sorting, or value-based sorting) and returns it.
func (r *Record) SortResults(results []string) []string {
	switch r.sortMode {
	case NoSort:
		return results
	case Rowsort:
		sorter := rowSorter{
			record: r,
			values: results,
		}
		sort.Sort(sorter)
		return sorter.values
	case ValueSort:
		sort.Strings(results)
		return results
	default:
		panic(fmt.Sprintf("Uncrecognized sort mode %v", r.sortMode))
	}
}

func (r *Record) SortString() string {
	return string(r.sortMode)
}

func (r *Record) Label() string {
	return r.label
}

func (r *Record) HashThreshold() int {
	return r.hashThreshold
}

