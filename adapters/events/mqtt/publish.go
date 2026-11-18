package mqtt

import (
	"strconv"
	"time"
)

// temperatureRoundingOffset rounds a temperature to the nearest whole degree.
// Subscribers expect whole degrees: 21.4 is published as 21 and 21.6 as 22.
const temperatureRoundingOffset = 0.5

// PublishState publishes the printer state machine value.
func (p *Publisher) PublishState(printerID, state string) {
	if state == "" {
		return
	}

	p.offer(DeviceState, DeviceState.For(printerID), state)
}

// PublishOnline publishes per-printer reachability.
func (p *Publisher) PublishOnline(printerID string, online bool) {
	p.offer(PrinterOnline, PrinterOnline.For(printerID), boolPayload(online))
}

// PublishSpeed publishes the print speed override percentage.
func (p *Publisher) PublishSpeed(printerID string, speed int) {
	p.offer(Speed, Speed.For(printerID), strconv.Itoa(speed))
}

// PublishAxis publishes one carriage position in millimetres.
func (p *Publisher) PublishAxis(printerID string, axis Topic, millimetres float64) {
	p.offer(axis, axis.For(printerID), strconv.FormatFloat(millimetres, 'f', 2, 64))
}

// PublishBlockingReasonsChanged tells subscribers to fetch a printer's
// blocking reasons again. The payload is the current unix time and contains
// no reasons. Subscribers ignore the body.
func (p *Publisher) PublishBlockingReasonsChanged(printerID string) {
	p.offer(BlockingReasons, BlockingReasons.For(printerID), strconv.FormatInt(time.Now().Unix(), 10))
}

// PublishCurrentJob publishes the printer-reported id of the running job.
// Zero means no job. It is published too, so a subscriber sees the job end.
func (p *Publisher) PublishCurrentJob(printerID string, jobID int) {
	p.offer(CurrentJob, CurrentJob.For(printerID), strconv.Itoa(jobID))
}

// PublishJobState publishes a job's state in the Connect vocabulary.
func (p *Publisher) PublishJobState(printerID string, jobID int, state string) {
	if jobID <= 0 || state == "" {
		return
	}

	p.offer(JobState, JobState.ForJob(printerID, jobID), state)
}

// PublishJobProgress publishes a percentage to both the printer-level and the
// job-level channel. A subscriber may watch either one.
func (p *Publisher) PublishJobProgress(printerID string, jobID int, percent float64) {
	payload := strconv.Itoa(int(percent))

	p.offer(JobProgress, JobProgress.For(printerID), payload)

	if jobID > 0 {
		p.offer(JobProgressByID, JobProgressByID.ForJob(printerID, jobID), payload)
	}
}

// PublishJobTimeRemaining publishes a job's estimate in seconds.
func (p *Publisher) PublishJobTimeRemaining(printerID string, jobID, seconds int) {
	if jobID <= 0 {
		return
	}

	p.offer(JobTimeRemaining, JobTimeRemaining.ForJob(printerID, jobID), strconv.Itoa(seconds))
}

// PublishDialog publishes a dialog object the caller already marshalled.
func (p *Publisher) PublishDialog(printerID, payloadJSON string) {
	p.offer(Dialog, Dialog.For(printerID), payloadJSON)
}

// PublishFact publishes one durable bus event on the fact channel, with a
// body the caller already marshalled. It skips the change filter: two
// identical facts are two things that happened.
func (p *Publisher) PublishFact(originPath, eventType, body string) {
	if originPath == "" || eventType == "" || body == "" {
		return
	}

	p.enqueue(FactObserved, FactObserved.ForFact(originPath, eventType), body)
}

func temperaturePayload(celsius float64) string {
	return strconv.Itoa(int(celsius + temperatureRoundingOffset))
}

func boolPayload(v bool) string {
	if v {
		return "1"
	}

	return "0"
}
