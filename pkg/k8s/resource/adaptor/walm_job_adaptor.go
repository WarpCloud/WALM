package adaptor

import (
	batchv1 "k8s.io/api/batch/v1"
	"fmt"
	"walm/pkg/k8s/handler"
)

type WalmJobAdaptor struct {
	jobHandler *handler.JobHandler
	podHandler *handler.PodHandler
}

func (adaptor WalmJobAdaptor) GetResource(namespace string, name string) (WalmResource, error) {
	job, err := adaptor.jobHandler.GetJob(namespace, name)
	if err != nil {
		if isNotFoundErr(err) {
			return WalmJob{
				WalmMeta: buildNotFoundWalmMeta("Job", namespace, name),
			}, nil
		}
		return WalmJob{}, err
	}

	return adaptor.BuildWalmJob(job)
}

func (adaptor WalmJobAdaptor) BuildWalmJob(job *batchv1.Job) (walmJob WalmJob, err error) {
	walmJob = WalmJob{
		WalmMeta: buildWalmMetaWithoutState("Job", job.Namespace, job.Name),
	}

	walmJob.Pods, err = WalmPodAdaptor{adaptor.podHandler}.GetWalmPods(job.Namespace, job.Spec.Selector)
	walmJob.State = BuildWalmJobState(job)
	return walmJob, err
}

func BuildWalmJobState(job *batchv1.Job) (jobState WalmState) {
	if len(job.Status.Conditions) > 0 {
		for _, condition := range job.Status.Conditions {
			if condition.Type == "Complete" && condition.Status == "True" {
				jobState = buildWalmState("Ready", "", "")
				break
			}
			if condition.Type == "Failed" && condition.Status == "True" {
				jobState = buildWalmState("Pending", condition.Reason, condition.Message)
				break
			}
		}
	} else if job.Status.Active > 0 {
		jobState = buildWalmState("Pending", "JobActive", fmt.Sprintf("There are %d active pod", job.Status.Active))
	} else {
		jobState = buildWalmState("Terminating", "", "")
	}

	return jobState
}
