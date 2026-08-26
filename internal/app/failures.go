package app

// failureLog decides how often a run of failing polls is worth mentioning. At
// two polls a second, logging every one turns an hour of Herdr being down into
// thousands of identical lines; logging as the run doubles costs a dozen.
type failureLog struct {
	run  int
	next int
}

// failed records a failed poll and returns the length of the run when it is
// worth logging, or zero when it is not.
func (f *failureLog) failed() int {
	f.run++
	if f.run < f.next {
		return 0
	}

	f.next = f.run * 2

	return f.run
}

// recovered records a successful poll and returns how many polls the run of
// failures it ended cost, or zero when nothing was wrong.
func (f *failureLog) recovered() int {
	run := f.run
	f.run, f.next = 0, 0

	return run
}
