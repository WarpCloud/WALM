package tenant

import (
	"WarpCloud/walm/pkg/k8s/handler"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"github.com/sirupsen/logrus"
	"fmt"
	"WarpCloud/walm/pkg/k8s/adaptor"
	walmerr "WarpCloud/walm/pkg/util/error"
	"k8s.io/apimachinery/pkg/api/resource"
	"WarpCloud/walm/pkg/util"
	"WarpCloud/walm/pkg/release/manager/helm"
	"sync"
)

const(
	limitRangeDefaultMem = "128Mi"
	limitRangeDefaultCpu = "0.1"
	LimitRangeDefaultName = "walm-default-limitrange"
)

func ListTenants() (TenantInfoList, error) {
	var tenantInfoList TenantInfoList

	namespaces, err := handler.GetDefaultHandlerSet().GetNamespaceHandler().ListNamespaces(nil)
	if err != nil {
		return tenantInfoList, err
	}
	for _, namespace := range namespaces {
		tenantInfo, err := GetTenant(namespace.Name)
		if err != nil {
			logrus.Errorf("ListTenants getTenant %s error %v", namespace.Name, err)
		}
		tenantInfoList.Items = append(tenantInfoList.Items, tenantInfo)
	}

	return tenantInfoList, nil
}

func GetTenant(tenantName string) (*TenantInfo, error) {
	namespace, err := handler.GetDefaultHandlerSet().GetNamespaceHandler().GetNamespace(tenantName)
	if err != nil {
		if adaptor.IsNotFoundErr(err) {
			return nil, walmerr.NotFoundError{}
		} else {
			return nil, err
		}
	}

	tenantInfo := TenantInfo{
		TenantName:            namespace.Name,
		TenantCreationTime:    namespace.CreationTimestamp,
		TenantLabels:          namespace.Labels,
		TenantAnnotitions:     namespace.Annotations,
		TenantStatus:          namespace.Status.String(),
		TenantQuotas:          []*TenantQuota{},
		UnifyUnitTenantQuotas: []*UnifyUnitTenantQuota{},
	}

	if tenantInfo.TenantLabels == nil {
		tenantInfo.TenantLabels = map[string]string{}
	}
	if tenantInfo.TenantAnnotitions == nil {
		tenantInfo.TenantAnnotitions = map[string]string{}
	}

	_, ok := namespace.Labels["multi-tenant"]
	if ok {
		tenantInfo.MultiTenant = true
	} else {
		tenantInfo.MultiTenant = false
	}

	if namespace.Status.Phase == corev1.NamespaceActive {
		tenantInfo.Ready = true
	}

	walmResourceQuotas, err := adaptor.GetDefaultAdaptorSet().GetAdaptor("ResourceQuota").(*adaptor.WalmResourceQuotaAdaptor).GetWalmResourceQuotas(tenantName, nil)
	if err != nil {
		logrus.Errorf("failed to get resource quotas : %s", err.Error())
		return nil, err
	}

	for _, walmResourceQuota := range walmResourceQuotas {
		hard := TenantQuotaInfo{
			Pods:            walmResourceQuota.ResourceLimits[corev1.ResourcePods],
			LimitCpu:        walmResourceQuota.ResourceLimits[corev1.ResourceLimitsCPU],
			LimitMemory:     walmResourceQuota.ResourceLimits[corev1.ResourceLimitsMemory],
			RequestsStorage: walmResourceQuota.ResourceLimits[corev1.ResourceRequestsStorage],
			RequestsMemory:  walmResourceQuota.ResourceLimits[corev1.ResourceRequestsMemory],
			RequestsCPU:     walmResourceQuota.ResourceLimits[corev1.ResourceRequestsCPU],
		}
		used := TenantQuotaInfo{
			Pods:            walmResourceQuota.ResourceUsed[corev1.ResourcePods],
			LimitCpu:        walmResourceQuota.ResourceUsed[corev1.ResourceLimitsCPU],
			LimitMemory:     walmResourceQuota.ResourceUsed[corev1.ResourceLimitsMemory],
			RequestsStorage: walmResourceQuota.ResourceUsed[corev1.ResourceRequestsStorage],
			RequestsMemory:  walmResourceQuota.ResourceUsed[corev1.ResourceRequestsMemory],
			RequestsCPU:     walmResourceQuota.ResourceUsed[corev1.ResourceRequestsCPU],
		}
		tenantInfo.TenantQuotas = append(tenantInfo.TenantQuotas, &TenantQuota{walmResourceQuota.Name, &hard, &used})
		tenantInfo.UnifyUnitTenantQuotas = append(tenantInfo.UnifyUnitTenantQuotas, buildUnifyUnitTenantQuota(walmResourceQuota.Name, hard, used))
	}

	return &tenantInfo, nil
}

func buildUnifyUnitTenantQuota(name string, hard TenantQuotaInfo, used TenantQuotaInfo) *UnifyUnitTenantQuota {
	return &UnifyUnitTenantQuota{
		QuotaName: name,
		Hard:      buildUnifyUnitTenantInfo(hard),
		Used:      buildUnifyUnitTenantInfo(used),
	}
}

func buildUnifyUnitTenantInfo(info TenantQuotaInfo) *UnifyUnitTenantQuotaInfo {
	return &UnifyUnitTenantQuotaInfo{
		RequestsCPU:     parseCpuValue(info.RequestsCPU),
		RequestsMemory:  parseMemoryValue(info.RequestsMemory),
		RequestsStorage: parseStorageValue(info.RequestsStorage),
		LimitMemory:     parseMemoryValue(info.LimitMemory),
		LimitCpu:        parseCpuValue(info.LimitCpu),
		Pods:            parsePodValue(info.Pods),
	}
}

func parseCpuValue(s string) float64 {
	quantity, err := resource.ParseQuantity(s)
	if err != nil {
		logrus.Warnf("failed to parse quantity %s : %s", s, err.Error())
	} else {
		return float64(quantity.MilliValue()) / util.K8sResourceCpuScale
	}
	return 0
}

func parseMemoryValue(s string) int64 {
	quantity, err := resource.ParseQuantity(s)
	if err != nil {
		logrus.Warnf("failed to parse quantity %s : %s", s, err.Error())
	} else {
		return quantity.Value() / util.K8sResourceMemoryScale
	}
	return 0
}

func parseStorageValue(s string) int64 {
	quantity, err := resource.ParseQuantity(s)
	if err != nil {
		logrus.Warnf("failed to parse quantity %s : %s", s, err.Error())
	} else {
		return quantity.Value() / util.K8sResourceStorageScale
	}
	return 0
}

func parsePodValue(s string) int64 {
	quantity, err := resource.ParseQuantity(s)
	if err != nil {
		logrus.Warnf("failed to parse quantity %s : %s", s, err.Error())
	} else {
		return quantity.Value()
	}
	return 0
}

// CreateTenant initialize the namespace for the tenant
// and installs the essential components
func CreateTenant(tenantName string, tenantParams *TenantParams) error {
	_, err := handler.GetDefaultHandlerSet().GetNamespaceHandler().GetNamespace(tenantName)
	if err != nil {
		if adaptor.IsNotFoundErr(err) {
			tenantLabel := make(map[string]string, 0)
			for k, v := range tenantParams.TenantLabels {
				tenantLabel[k] = v
			}
			tenantLabel["multi-tenant"] = fmt.Sprintf("tenant-tiller-%s", tenantName)
			namespace := corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Namespace:   tenantName,
					Name:        tenantName,
					Labels:      tenantLabel,
					Annotations: tenantParams.TenantAnnotations,
				},
			}
			_, err = handler.GetDefaultHandlerSet().GetNamespaceHandler().CreateNamespace(&namespace)
			if err != nil {
				logrus.Errorf("failed to create namespace %s : %s", tenantName, err.Error())
				return err
			}

			err = doCreateTenant(tenantName, tenantParams)
			if err != nil {
				// rollback
				handler.GetDefaultHandlerSet().GetNamespaceHandler().DeleteNamespace(tenantName)
				return err
			}
			logrus.Infof("succeed to create tenant %s", tenantName)
			return nil
		}
		logrus.Errorf("failed to get namespace : %s", err.Error())
		return err

	} else {
		logrus.Warnf("namespace %s exists", tenantName)
		return nil
	}
}

func doCreateTenant(tenantName string, tenantParams *TenantParams) error {
	for _, tenantQuota := range tenantParams.TenantQuotas {
		err := createResourceQuota(tenantName, tenantQuota)
		if err != nil {
			logrus.Errorf("failed to create resource quota : %s", err.Error())
			return err
		}
	}

	handler.GetDefaultHandlerSet().GetLimitRangeHandler().CreateLimitRange(tenantName, getDefaultLimitRange())

	return nil
}

func getDefaultLimitRange() *corev1.LimitRange {
	return &corev1.LimitRange{
		ObjectMeta: metav1.ObjectMeta{
			Name: LimitRangeDefaultName,
		},
		Spec: corev1.LimitRangeSpec{
			Limits: []corev1.LimitRangeItem{
				{
					Type: corev1.LimitTypeContainer,
					Default: corev1.ResourceList{
						corev1.ResourceMemory: resource.MustParse(limitRangeDefaultMem),
						corev1.ResourceCPU: resource.MustParse(limitRangeDefaultCpu),
					},
				},
			},
		},
	}
}

func createResourceQuota(tenantName string, tenantQuota *TenantQuotaParams) error {
	walmResourceQuota := adaptor.WalmResourceQuota{
		WalmMeta: adaptor.WalmMeta{
			Namespace: tenantName,
			Name:      tenantQuota.QuotaName,
			Kind:      "ResourceQuota",
		},
		ResourceLimits: map[corev1.ResourceName]string{
			corev1.ResourcePods:            tenantQuota.Hard.Pods,
			corev1.ResourceLimitsCPU:       tenantQuota.Hard.LimitCpu,
			corev1.ResourceLimitsMemory:    tenantQuota.Hard.LimitMemory,
			corev1.ResourceRequestsCPU:     tenantQuota.Hard.RequestsCPU,
			corev1.ResourceRequestsMemory:  tenantQuota.Hard.RequestsMemory,
			corev1.ResourceRequestsStorage: tenantQuota.Hard.RequestsStorage,
		},
	}

	resourceQuota, err := adaptor.BuildResourceQuota(&walmResourceQuota)
	if err != nil {
		logrus.Errorf("failed to build resource quota : %s", err.Error())
		return err
	}
	_, err = handler.GetDefaultHandlerSet().GetResourceQuotaHandler().CreateResourceQuota(tenantName, resourceQuota)
	if err != nil {
		logrus.Errorf("failed to create resource quota : %s", err.Error())
		return err
	}
	return nil
}

func DeleteTenant(tenantName string) error {
	_, err := handler.GetDefaultHandlerSet().GetNamespaceHandler().GetNamespace(tenantName)
	if err != nil {
		if adaptor.IsNotFoundErr(err) {
			return nil
		} else {
			return err
		}
	}

	releases, err := helm.GetDefaultHelmClient().ListReleases(tenantName, "")
	if err != nil {
		logrus.Errorf("failed to get releases in tenant %s : %s", tenantName, err.Error())
		return err
	}

	var wg sync.WaitGroup
	for _, release := range releases {
		wg.Add(1)
		go func(releaseName string) {
			defer wg.Done()
			err1 := helm.GetDefaultHelmClient().DeleteReleaseWithRetry(tenantName, releaseName, false, false, false, 0)
			if err1 != nil {
				err = fmt.Errorf("failed to delete release %s under tenant %s : %s", releaseName, tenantName, err1.Error())
				logrus.Error(err.Error())
			}
		}(release.Name)
	}
	wg.Wait()

	if err != nil {
		return err
	}

	err = handler.GetDefaultHandlerSet().GetNamespaceHandler().DeleteNamespace(tenantName)
	if err != nil {
		logrus.Errorf("failed to delete namespace : %s", err.Error())
		return err
	}

	logrus.Infof("succeed to delete tenant %s", tenantName)
	return nil
}

func UpdateTenant(tenantName string, tenantParams *TenantParams) error {
	namespace, err := handler.GetDefaultHandlerSet().GetNamespaceHandler().GetNamespace(tenantName)
	if err != nil {
		logrus.Errorf("failed to get namespace : %s", err.Error())
		return err
	}
	if len(tenantParams.TenantAnnotations) > 0 {
		if namespace.Annotations == nil {
			namespace.Annotations = map[string]string{}
		}
		for key, value := range tenantParams.TenantAnnotations {
			namespace.Annotations[key] = value
		}
	}

	if len(tenantParams.TenantLabels) > 0 {
		if namespace.Labels == nil {
			namespace.Labels = map[string]string{}
		}
		for key, value := range tenantParams.TenantLabels {
			namespace.Labels[key] = value
		}
	}

	_, err = handler.GetDefaultHandlerSet().GetNamespaceHandler().UpdateNamespace(namespace)
	if err != nil {
		logrus.Errorf("failed to update namespace : %s", err.Error())
		return err
	}

	if len(tenantParams.TenantQuotas) > 0 {
		resourceQuotas, err := handler.GetDefaultHandlerSet().GetResourceQuotaHandler().ListResourceQuota(tenantName, nil)
		if err != nil {
			logrus.Errorf("failed to get resource quotas : %s", err.Error())
			return err
		}

		resourceQuotaMap := map[string]*corev1.ResourceQuota{}
		for _, resourceQuota := range resourceQuotas {
			resourceQuotaMap[resourceQuota.Name] = resourceQuota
		}

		for _, tenantQuota := range tenantParams.TenantQuotas {
			if resourceQuota, ok := resourceQuotaMap[tenantQuota.QuotaName]; ok {
				hard := map[corev1.ResourceName]string{
					corev1.ResourcePods:            tenantQuota.Hard.Pods,
					corev1.ResourceLimitsCPU:       tenantQuota.Hard.LimitCpu,
					corev1.ResourceLimitsMemory:    tenantQuota.Hard.LimitMemory,
					corev1.ResourceRequestsCPU:     tenantQuota.Hard.RequestsCPU,
					corev1.ResourceRequestsMemory:  tenantQuota.Hard.RequestsMemory,
					corev1.ResourceRequestsStorage: tenantQuota.Hard.RequestsStorage,
				}

				for key, value := range hard {
					resourceQuota.Spec.Hard[key], err = resource.ParseQuantity(value)
					if err != nil {
						logrus.Errorf("failed to parse quota quantity %s: %s", value, err.Error())
						return err
					}
				}

				_, err = handler.GetDefaultHandlerSet().GetResourceQuotaHandler().UpdateResourceQuota(tenantName, resourceQuota)
				if err != nil {
					logrus.Errorf("failed to update resource quota %s: %s", tenantQuota.QuotaName, err.Error())
					return err
				}
			} else {
				err = createResourceQuota(tenantName, tenantQuota)
				if err != nil {
					logrus.Errorf("failed to create resource quota : %s", err.Error())
					return err
				}
			}
		}
	}

	logrus.Infof("succeed to update tenant %s", tenantName)
	return nil
}
