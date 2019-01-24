package helm

import (
	"walm/pkg/release"
	"github.com/sirupsen/logrus"
	"fmt"
	"walm/pkg/k8s/adaptor"
	"reflect"
)

func buildReleaseInfo(releaseCache *release.ReleaseCache) (releaseInfo *release.ReleaseInfo, err error) {
	releaseInfo = &release.ReleaseInfo{}
	releaseInfo.ReleaseSpec = releaseCache.ReleaseSpec

	releaseInfo.Status, err = buildReleaseStatus(releaseCache.ReleaseResourceMetas)
	if err != nil {
		logrus.Errorf(fmt.Sprintf("Failed to build the status of releaseInfo: %s", releaseInfo.Name))
		return
	}
	ready, notReadyResource := releaseInfo.Status.IsReady()
	if ready {
		releaseInfo.Ready = true
	} else {
		releaseInfo.Message = fmt.Sprintf("%s %s/%s is in state %s", notReadyResource.GetKind(), notReadyResource.GetNamespace(), notReadyResource.GetName(), notReadyResource.GetState().Status)
	}

	return
}

func buildReleaseStatus(releaseResourceMetas []release.ReleaseResourceMeta) (resourceSet *adaptor.WalmResourceSet,err error) {
	resourceSet = adaptor.NewWalmResourceSet()
	for _, resourceMeta := range releaseResourceMetas {
		resource, err := adaptor.GetDefaultAdaptorSet().GetAdaptor(resourceMeta.Kind).GetResource(resourceMeta.Namespace, resourceMeta.Name)
		if err != nil {
			return nil, err
		}
		resource.AddToWalmResourceSet(resourceSet)
	}
	return
}

func mergeValues(dest map[string]interface{}, src map[string]interface{}) map[string]interface{} {
	for k, v := range src {
		// If the key doesn't exist already, then just set the key to that value
		if _, exists := dest[k]; !exists {
			dest[k] = v
			continue
		}
		nextMap, ok := v.(map[string]interface{})
		// If it isn't another map, overwrite the value
		if !ok {
			dest[k] = v
			continue
		}
		// Edge case: If the key exists in the destination, but isn't a map
		destMap, isMap := dest[k].(map[string]interface{})
		// If the source map has a map for this key, prefer it
		if !isMap {
			dest[k] = v
			continue
		}
		// If we got to this point, it is a map in both, so merge them
		dest[k] = mergeValues(destMap, nextMap)
	}
	return dest
}

func ConfigValuesDiff(configValue1 map[string]interface{}, configValue2 map[string]interface{}) bool {
	if len(configValue1) == 0 && len(configValue2) == 0 {
		return false
	}
	return !reflect.DeepEqual(configValue1, configValue2)
}
