package controller

import (
	"fmt"
	"github.com/julienschmidt/httprouter"
	"net/http"
	"pannetrat.com/nocan/model"
	"pannetrat.com/nocan/view"
	"strconv"
)

type JobController struct {
}

func NewJobController() *JobController {
	controller := &JobController{}
	return controller
}

func (jc *JobController) GetJobId(w http.ResponseWriter, jobIdString string) *model.JobState {
	jobId, err := strconv.ParseUint(jobIdString, 10, 32)
	if err != nil {
		view.LogHttpError(w, "Could not understand job "+jobIdString, http.StatusBadRequest)
		return nil
	}

	job := model.Jobs.FindJob(uint(jobId))
	if job == nil {
		view.LogHttpError(w, "Could not find job "+jobIdString, http.StatusNotFound)
		return nil
	}

	return job
}

func (jc *JobController) Show(w http.ResponseWriter, r *http.Request, params httprouter.Params) {
	jobIdString := params.ByName("id")
	job := jc.GetJobId(w, jobIdString)
	if job == nil {
		return
	}

	switch job.GetStatus() {
	case model.JobStarted:
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, "%d", job.GetProgress())

	case model.JobCompleted:
		if job.Result != nil {
			w.Header().Set("Location", r.RequestURI+"/result")
		} else {
			model.Jobs.FinalizeJob(job.Id)
		}
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, "done")

	case model.JobFailed:
		view.LogHttpError(w, fmt.Sprintf("Job %d failed, %s", job.Id, job.FailureReason.Error()), http.StatusServiceUnavailable)
		model.Jobs.FinalizeJob(job.Id)
	}
}

func (jc *JobController) Result(w http.ResponseWriter, r *http.Request, params httprouter.Params) {
	jobIdString := params.ByName("id")
	job := jc.GetJobId(w, jobIdString)
	if job == nil {
		return
	}
	w.WriteHeader(http.StatusOK)
	if job.Result != nil {
		w.Write(job.Result)
	}
	model.Jobs.FinalizeJob(job.Id)
}
