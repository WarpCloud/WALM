package pvc

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"WarpCloud/walm/pkg/release"
	"github.com/satori/go.uuid"
	"WarpCloud/walm/pkg/k8s/handler"
	"go/build"
	"io/ioutil"
	"os"
	"encoding/json"
	"WarpCloud/walm/pkg/release/manager/helm"
	"WarpCloud/walm/pkg/k8s/adaptor"
	"k8s.io/apimachinery/pkg/apis/meta/v1"
	"github.com/sirupsen/logrus"
	"time"
)

var _ = Describe("Pvc", func() {

	var (
		namespace         string
		gopath            string
		releaseName       string
		releaseInfo       *release.ReleaseInfoV2
		labelSelector     *v1.LabelSelector
		labelSelectorStrs []string
		releaseRequest    release.ReleaseRequestV2
		err               error
	)

	BeforeEach(func() {

		By("create namespace")
		randomId := uuid.Must(uuid.NewV4()).String()
		namespace = "test-" + randomId[:8]

		ns := corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: namespace,
				Name:      namespace,
			},
		}
		_, err = handler.GetDefaultHandlerSet().GetNamespaceHandler().CreateNamespace(&ns)
		Expect(err).NotTo(HaveOccurred())

		By("create a release")

		gopath = os.Getenv("GOPATH")
		if gopath == "" {
			gopath = build.Default.GOPATH
		}
		releaseChartByte, err := ioutil.ReadFile(gopath + "/src/walm/test/resources/releases/weblogic.yaml")
		Expect(err).NotTo(HaveOccurred())

		err = json.Unmarshal(releaseChartByte, &releaseRequest)
		Expect(err).NotTo(HaveOccurred())

		releaseRequest.Name = releaseRequest.Name + "-" + randomId[:8]
		releaseName = releaseRequest.Name
		err = helm.GetDefaultHelmClient().InstallUpgradeRelease(namespace, &releaseRequest, false, nil, false, 0)
		Expect(err).NotTo(HaveOccurred())

		By("check release status")

		finish := make(chan bool)
		timeout := time.After(time.Second * 720)

		go func() {
			for {
				select {
				case <-timeout:
					Fail("install release timeout, check out please")
				default:
					releaseInfo, err = helm.GetDefaultHelmClient().GetRelease(namespace, releaseName)
					Expect(err).NotTo(HaveOccurred())
					logrus.Infof("install release status: ongoing")
					if releaseInfo.Ready {
						logrus.Infof("install release ready")
						finish <- true
						return
					}
				}
				time.Sleep(time.Second * 20)
			}
		}()

		<-finish

	})

	AfterEach(func() {

		By("delete release")
		err := helm.GetDefaultHelmClient().DeleteRelease(namespace, releaseName, false, true, false, 0)
		Expect(err).NotTo(HaveOccurred())

		By("delete namespace")
		err = handler.GetDefaultHandlerSet().GetNamespaceHandler().DeleteNamespace(namespace)
		Expect(err).NotTo(HaveOccurred())

	})

	Describe("delete pvcs with sts", func() {
		It("delete pvcs with sts exists", func() {

			By("get release sts")
			releaseInfo, err := helm.GetDefaultHelmClient().GetRelease(namespace, releaseName)
			Expect(err).NotTo(HaveOccurred())

			for _, statefulset := range releaseInfo.Status.StatefulSets {
				labelSelectorStr := ""
				for k, v := range statefulset.Labels {
					if k == "app.kubernetes.io/name" {
						labelSelectorStr += k + "=" + v
					}
				}
				labelSelectorStrs = append(labelSelectorStrs, labelSelectorStr)
			}

			By("delete pvc with sts exists failed")

			for _, labelSelectorStr := range labelSelectorStrs {

				labelSelector, err = metav1.ParseToLabelSelector(labelSelectorStr)
				Expect(err).NotTo(HaveOccurred())

				pvcAdaptor := adaptor.GetDefaultAdaptorSet().GetAdaptor("PersistentVolumeClaim").(*adaptor.WalmPersistentVolumeClaimAdaptor)
				pvcs, err := pvcAdaptor.GetWalmPersistentVolumeClaimAdaptors(namespace, labelSelector)
				Expect(err).NotTo(HaveOccurred())

				expectPvcs := len(pvcs)

				for _, pvc := range pvcs {
					err = pvcAdaptor.DeletePvc(namespace, pvc.Name)
					if err != nil {
						if adaptor.IsNotFoundErr(err) {
							logrus.Warnf("pvc %s/%s is not found", namespace, pvc.Name)
							continue
						}
						logrus.Warnf("%v", err)
					}
				}
				pvcs, err = pvcAdaptor.GetWalmPersistentVolumeClaimAdaptors(namespace, labelSelector)
				Expect(err).NotTo(HaveOccurred())
				Expect(len(pvcs)).To(Equal(expectPvcs))
			}
		})
	})
})