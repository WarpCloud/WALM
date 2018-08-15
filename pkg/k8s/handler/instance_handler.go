package handler

import (
	clientsetex "transwarp/application-instance/pkg/client/clientset/versioned"
	"transwarp/application-instance/pkg/apis/transwarp/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sutils "walm/pkg/k8s/utils"
	listv1beta1 "transwarp/application-instance/pkg/client/listers/transwarp/v1beta1"
)

type InstanceHandler struct {
	client *clientsetex.Clientset
	lister listv1beta1.ApplicationInstanceLister
}

func (handler *InstanceHandler) GetInstance(namespace string, name string) (*v1beta1.ApplicationInstance, error) {
	return handler.lister.ApplicationInstances(namespace).Get(name)
}

func (handler *InstanceHandler) ListInstances(namespace string, labelSelector *metav1.LabelSelector) ([]*v1beta1.ApplicationInstance, error) {
	selector, err := k8sutils.ConvertLabelSelectorToSelector(labelSelector)
	if err != nil {
		return nil, err
	}
	return handler.lister.ApplicationInstances(namespace).List(selector)
}

