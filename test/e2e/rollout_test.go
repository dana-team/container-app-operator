package e2e

import (
	"context"
	"fmt"
	"time"

	cappv1alpha1 "github.com/dana-team/container-app-operator/api/v1alpha1"
	"github.com/dana-team/container-app-operator/test/e2e/consts"
	"github.com/dana-team/container-app-operator/test/e2e/mocks"
	"github.com/dana-team/container-app-operator/test/e2e/utils"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// newRolloutCapp returns a base Capp with the given rollout strategy.
func newRolloutCapp(strategy *cappv1alpha1.RolloutStrategy) *cappv1alpha1.Capp {
	capp := mocks.CreateBaseCapp()
	capp.Spec.RouteSpec.RolloutStrategy = strategy
	return capp
}

// stage returns a rollout stage; a zero duration means the stage has no duration.
func stage(percent int64, duration *metav1.Duration) cappv1alpha1.RolloutStage {
	return cappv1alpha1.RolloutStage{Percent: percent, Duration: duration}
}

// stageDuration returns the e2e stage duration.
func stageDuration() *metav1.Duration {
	return &metav1.Duration{Duration: consts.RolloutStageDuration}
}

// rolloutStatus returns the rollout status of a Capp, or an empty one when unset.
func rolloutStatus(name, namespace string) cappv1alpha1.RolloutStatus {
	capp := utils.GetCapp(k8sClient, name, namespace)
	if capp.Status.Rollout == nil {
		return cappv1alpha1.RolloutStatus{}
	}
	return *capp.Status.Rollout
}

// waitForStableRevision waits for the first revision to become the stable one and returns its name.
func waitForStableRevision(name, namespace string) string {
	GinkgoHelper()
	Eventually(func() cappv1alpha1.RolloutPhase {
		return rolloutStatus(name, namespace).Phase
	}, consts.Timeout, consts.Interval).Should(Equal(cappv1alpha1.RolloutPhaseStable))

	stable := rolloutStatus(name, namespace).StableRevision
	Expect(stable).ToNot(BeEmpty())
	return stable
}

// conditionStatus returns the status of a Capp condition, or Unknown when the condition is not set.
func conditionStatus(name, namespace, conditionType string) metav1.ConditionStatus {
	condition := meta.FindStatusCondition(utils.GetCapp(k8sClient, name, namespace).Status.Conditions, conditionType)
	if condition == nil {
		return metav1.ConditionUnknown
	}
	return condition.Status
}

// ksvcTraffic returns the percent per revision name in the Knative Service traffic.
func ksvcTraffic(name, namespace string) map[string]int64 {
	traffic := map[string]int64{}
	for _, target := range utils.GetKSVC(k8sClient, name, namespace).Spec.Traffic {
		if target.Percent != nil {
			traffic[target.RevisionName] += *target.Percent
		}
	}
	return traffic
}

// cappHasEvent reports whether an event with the given reason exists for the Capp.
func cappHasEvent(name, namespace, reason string) bool {
	eventList := &corev1.EventList{}
	if err := k8sClient.List(context.Background(), eventList, client.InNamespace(namespace)); err != nil {
		return false
	}
	for _, event := range eventList.Items {
		if event.InvolvedObject.Name == name && event.Reason == reason {
			return true
		}
	}
	return false
}

var _ = Describe("Validate Capp rollouts", func() {
	It("Should run a canary rollout and make the new revision stable", func() {
		createdCapp := utils.CreateCapp(Default, k8sClient, newRolloutCapp(&cappv1alpha1.RolloutStrategy{
			Stages: []cappv1alpha1.RolloutStage{stage(10, stageDuration()), stage(100, nil)},
		}))
		oldRevision := waitForStableRevision(createdCapp.Name, createdCapp.Namespace)

		By("Updating the image to create a new revision")
		utils.UpdateCappImage(k8sClient, createdCapp.Name, createdCapp.Namespace, consts.ImageExample)

		By("Checking the new revision receives 10% and the old one 90%")
		var newRevision string
		Eventually(func(g Gomega) {
			newRevision = rolloutStatus(createdCapp.Name, createdCapp.Namespace).CandidateRevision
			g.Expect(newRevision).ToNot(BeEmpty())
			g.Expect(newRevision).ToNot(Equal(oldRevision))
			g.Expect(ksvcTraffic(createdCapp.Name, createdCapp.Namespace)).To(And(
				HaveLen(2),
				HaveKeyWithValue(oldRevision, int64(90)),
				HaveKeyWithValue(newRevision, int64(10)),
			))
		}, consts.Timeout, consts.Interval).Should(Succeed())

		By("Checking the rollout succeeds and the new revision is stable")
		Eventually(func() metav1.ConditionStatus {
			return conditionStatus(createdCapp.Name, createdCapp.Namespace, cappv1alpha1.CappConditionRolloutSucceeded)
		}, consts.Timeout, consts.Interval).Should(Equal(metav1.ConditionTrue))
		Expect(rolloutStatus(createdCapp.Name, createdCapp.Namespace).StableRevision).To(Equal(newRevision))
		Expect(ksvcTraffic(createdCapp.Name, createdCapp.Namespace)).To(HaveKeyWithValue(newRevision, int64(100)))
	})

	It("Should restart the rollout when another revision arrives during a stage", func() {
		createdCapp := utils.CreateCapp(Default, k8sClient, newRolloutCapp(&cappv1alpha1.RolloutStrategy{
			Stages: []cappv1alpha1.RolloutStage{stage(10, stageDuration()), stage(50, nil)},
		}))
		stable := waitForStableRevision(createdCapp.Name, createdCapp.Namespace)

		By("Creating the second revision")
		utils.UpdateCappImage(k8sClient, createdCapp.Name, createdCapp.Namespace, consts.ImageExample)
		var secondRevision string
		Eventually(func() string {
			secondRevision = rolloutStatus(createdCapp.Name, createdCapp.Namespace).CandidateRevision
			return secondRevision
		}, consts.Timeout, consts.Interval).ShouldNot(BeElementOf("", stable))

		By("Creating a third revision during stage 0")
		utils.UpdateCappImage(k8sClient, createdCapp.Name, createdCapp.Namespace, consts.ImageExampleLatest)

		By("Checking the candidate becomes the third revision and the stable one is unchanged")
		Eventually(func() string {
			return rolloutStatus(createdCapp.Name, createdCapp.Namespace).CandidateRevision
		}, consts.Timeout, consts.Interval).ShouldNot(BeElementOf("", stable, secondRevision))
		Expect(rolloutStatus(createdCapp.Name, createdCapp.Namespace).StableRevision).To(Equal(stable))

		By("Checking a RolloutRestarted event was emitted")
		Eventually(func() bool {
			return cappHasEvent(createdCapp.Name, createdCapp.Namespace, "RolloutRestarted")
		}, consts.Timeout, consts.Interval).Should(BeTrue())
	})

	It("Should roll back automatically when the candidate never becomes ready", func() {
		createdCapp := utils.CreateCapp(Default, k8sClient, newRolloutCapp(&cappv1alpha1.RolloutStrategy{
			Stages:           []cappv1alpha1.RolloutStage{stage(10, stageDuration()), stage(100, nil)},
			AutoRollback:     true,
			ProgressDeadline: &metav1.Duration{Duration: consts.RolloutProgressDeadline},
		}))
		stable := waitForStableRevision(createdCapp.Name, createdCapp.Namespace)

		By("Updating the image to an unpullable one")
		utils.UpdateCappImage(k8sClient, createdCapp.Name, createdCapp.Namespace, consts.UnpullableImage)

		By("Checking the phase becomes RolledBack and the stable revision gets 100%")
		Eventually(func() cappv1alpha1.RolloutPhase {
			return rolloutStatus(createdCapp.Name, createdCapp.Namespace).Phase
		}, consts.Timeout, consts.Interval).Should(Equal(cappv1alpha1.RolloutPhaseRolledBack))
		Eventually(func() map[string]int64 {
			return ksvcTraffic(createdCapp.Name, createdCapp.Namespace)
		}, consts.Timeout, consts.Interval).Should(HaveKeyWithValue(stable, int64(100)))

		By("Checking the revisions serving traffic are available and the rollout did not succeed")
		Eventually(func() metav1.ConditionStatus {
			return conditionStatus(createdCapp.Name, createdCapp.Namespace, cappv1alpha1.CappConditionAvailable)
		}, consts.Timeout, consts.Interval).Should(Equal(metav1.ConditionTrue))
		Eventually(func() metav1.ConditionStatus {
			return conditionStatus(createdCapp.Name, createdCapp.Namespace, cappv1alpha1.CappConditionRolloutSucceeded)
		}, consts.Timeout, consts.Interval).Should(Equal(metav1.ConditionFalse))
	})

	It("Should retry an aborted rollout of the same revision", func() {
		createdCapp := utils.CreateCapp(Default, k8sClient, newRolloutCapp(&cappv1alpha1.RolloutStrategy{
			// The stage must outlast the abort below: once it ends the rollout promotes and the abort is rejected.
			Stages: []cappv1alpha1.RolloutStage{stage(10, &metav1.Duration{Duration: 2 * consts.RolloutStageDuration}), stage(100, nil)},
		}))
		stable := waitForStableRevision(createdCapp.Name, createdCapp.Namespace)

		By("Updating the image and waiting for the candidate to receive 10%")
		utils.UpdateCappImage(k8sClient, createdCapp.Name, createdCapp.Namespace, consts.ImageExample)
		var candidate string
		Eventually(func(g Gomega) {
			candidate = rolloutStatus(createdCapp.Name, createdCapp.Namespace).CandidateRevision
			g.Expect(candidate).ToNot(BeEmpty())
			g.Expect(ksvcTraffic(createdCapp.Name, createdCapp.Namespace)).To(HaveKeyWithValue(candidate, int64(10)))
		}, consts.Timeout, consts.Interval).Should(Succeed())

		By("Aborting the candidate")
		utils.AnnotateCapp(k8sClient, createdCapp.Name, createdCapp.Namespace, consts.RolloutAnnotationKey,
			fmt.Sprintf("%s:%s", consts.RolloutAbortAction, candidate))
		Eventually(func() cappv1alpha1.RolloutPhase {
			return rolloutStatus(createdCapp.Name, createdCapp.Namespace).Phase
		}, consts.Timeout, consts.Interval).Should(Equal(cappv1alpha1.RolloutPhaseAborted))
		Eventually(func() map[string]int64 {
			return ksvcTraffic(createdCapp.Name, createdCapp.Namespace)
		}, consts.Timeout, consts.Interval).Should(HaveKeyWithValue(stable, int64(100)))

		By("Retrying the candidate and checking it restarts at 10%")
		utils.AnnotateCapp(k8sClient, createdCapp.Name, createdCapp.Namespace, consts.RolloutAnnotationKey,
			fmt.Sprintf("%s:%s", consts.RolloutRetryAction, candidate))
		Eventually(func() map[string]int64 {
			return ksvcTraffic(createdCapp.Name, createdCapp.Namespace)
		}, consts.Timeout, consts.Interval).Should(HaveKeyWithValue(candidate, int64(10)))

		By("Checking the retried rollout succeeds and the candidate becomes stable")
		Eventually(func() metav1.ConditionStatus {
			return conditionStatus(createdCapp.Name, createdCapp.Namespace, cappv1alpha1.CappConditionRolloutSucceeded)
		}, consts.Timeout, consts.Interval).Should(Equal(metav1.ConditionTrue))
		Expect(rolloutStatus(createdCapp.Name, createdCapp.Namespace).StableRevision).To(Equal(candidate))
	})

	It("Should keep traffic on the stable revision when the strategy is removed after a rollback", func() {
		createdCapp := utils.CreateCapp(Default, k8sClient, newRolloutCapp(&cappv1alpha1.RolloutStrategy{
			Stages:           []cappv1alpha1.RolloutStage{stage(10, stageDuration()), stage(100, nil)},
			AutoRollback:     true,
			ProgressDeadline: &metav1.Duration{Duration: consts.RolloutProgressDeadline},
		}))
		stable := waitForStableRevision(createdCapp.Name, createdCapp.Namespace)

		By("Updating the image to an unpullable one and waiting for the rollback")
		utils.UpdateCappImage(k8sClient, createdCapp.Name, createdCapp.Namespace, consts.UnpullableImage)
		Eventually(func() cappv1alpha1.RolloutPhase {
			return rolloutStatus(createdCapp.Name, createdCapp.Namespace).Phase
		}, consts.Timeout, consts.Interval).Should(Equal(cappv1alpha1.RolloutPhaseRolledBack))

		By("Removing the rollout strategy")
		utils.ClearRolloutStrategy(k8sClient, createdCapp.Name, createdCapp.Namespace)

		By("Checking the stable revision is pinned and keeps 100% of the traffic")
		Eventually(func() string {
			return rolloutStatus(createdCapp.Name, createdCapp.Namespace).PinnedRevision
		}, consts.Timeout, consts.Interval).Should(Equal(stable))
		Eventually(func() map[string]int64 {
			return ksvcTraffic(createdCapp.Name, createdCapp.Namespace)
		}, consts.Timeout, consts.Interval).Should(Equal(map[string]int64{stable: 100}))
		Consistently(func() map[string]int64 {
			return ksvcTraffic(createdCapp.Name, createdCapp.Namespace)
		}, 30*time.Second, consts.Interval).Should(Equal(map[string]int64{stable: 100}))

		By("Checking a RolloutPinned event was emitted")
		Eventually(func() bool {
			return cappHasEvent(createdCapp.Name, createdCapp.Namespace, "RolloutPinned")
		}, consts.Timeout, consts.Interval).Should(BeTrue())
	})

	It("Should promote a blue-green candidate when annotated", func() {
		createdCapp := utils.CreateCapp(Default, k8sClient, newRolloutCapp(&cappv1alpha1.RolloutStrategy{
			Stages: []cappv1alpha1.RolloutStage{stage(0, nil)},
		}))
		stable := waitForStableRevision(createdCapp.Name, createdCapp.Namespace)

		By("Updating the image and waiting for the rollout to hold at 0%")
		utils.UpdateCappImage(k8sClient, createdCapp.Name, createdCapp.Namespace, consts.ImageExample)
		var candidate string
		Eventually(func() cappv1alpha1.RolloutPhase {
			status := rolloutStatus(createdCapp.Name, createdCapp.Namespace)
			candidate = status.CandidateRevision
			return status.Phase
		}, consts.Timeout, consts.Interval).Should(Equal(cappv1alpha1.RolloutPhaseHolding))
		Expect(candidate).ToNot(BeElementOf("", stable))
		Expect(ksvcTraffic(createdCapp.Name, createdCapp.Namespace)).To(HaveKeyWithValue(stable, int64(100)))

		By("Annotating the Capp with promote for the candidate revision")
		utils.AnnotateCapp(k8sClient, createdCapp.Name, createdCapp.Namespace, consts.RolloutAnnotationKey,
			fmt.Sprintf("%s:%s", consts.RolloutPromoteAction, candidate))

		By("Checking the candidate gets 100% and becomes stable")
		Eventually(func() map[string]int64 {
			return ksvcTraffic(createdCapp.Name, createdCapp.Namespace)
		}, consts.Timeout, consts.Interval).Should(HaveKeyWithValue(candidate, int64(100)))
		Eventually(func() string {
			return rolloutStatus(createdCapp.Name, createdCapp.Namespace).StableRevision
		}, consts.Timeout, consts.Interval).Should(Equal(candidate))

		By("Checking the annotation was removed")
		Eventually(func() bool {
			_, found := utils.GetCapp(k8sClient, createdCapp.Name, createdCapp.Namespace).Annotations[consts.RolloutAnnotationKey]
			return found
		}, consts.Timeout, consts.Interval).Should(BeFalse())
	})

	It("Should keep routing 100% to the latest revision when no strategy is set", func() {
		createdCapp := utils.CreateCapp(Default, k8sClient, mocks.CreateBaseCapp())

		Eventually(func() bool {
			return utils.GetKSVC(k8sClient, createdCapp.Name, createdCapp.Namespace).Status.LatestReadyRevisionName != ""
		}, consts.Timeout, consts.Interval).Should(BeTrue())

		By("Checking the Knative Service traffic is the Knative default")
		ksvc := utils.GetKSVC(k8sClient, createdCapp.Name, createdCapp.Namespace)
		if len(ksvc.Spec.Traffic) > 0 {
			Expect(ksvc.Spec.Traffic).To(HaveLen(1))
			Expect(ksvc.Spec.Traffic[0].LatestRevision).To(HaveValue(BeTrue()))
			Expect(ksvc.Spec.Traffic[0].Percent).To(HaveValue(BeEquivalentTo(100)))
		}
		Expect(ksvc.Status.Traffic).ToNot(BeEmpty())
		Expect(ksvc.Status.Traffic[0].Percent).To(HaveValue(BeEquivalentTo(100)))
	})
})
