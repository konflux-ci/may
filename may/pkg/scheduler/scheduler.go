package scheduler

import (
	"context"
	"errors"
	"fmt"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	maykonfluxcidevv1alpha1 "github.com/konflux-ci/may/api/v1alpha1"
	"github.com/konflux-ci/may/pkg/claim"
	"github.com/konflux-ci/may/pkg/constants"
	"github.com/konflux-ci/may/pkg/runner"
)

var (
	ErrNoAvailableRunner   error = fmt.Errorf("no available runner")
	ErrClaimNotSchedulable error = fmt.Errorf("claim not schedulable")
)

func IsNoAvailableRunner(err error) bool {
	return errors.Is(ErrNoAvailableRunner, err)
}

func IsClaimNotSchedulable(err error) bool {
	return errors.Is(ErrClaimNotSchedulable, err)
}

type Scheduler struct {
	client.Client
	scheme    *runtime.Scheme
	namespace string
}

func New(cli client.Client, scheme *runtime.Scheme, namespace string) Scheduler {
	return Scheduler{
		Client:    cli,
		scheme:    scheme,
		namespace: namespace,
	}
}

func (s *Scheduler) Schedule(
	ctx context.Context,
	c maykonfluxcidevv1alpha1.Claim,
) (*maykonfluxcidevv1alpha1.Runner, error) {
	if !claim.IsPending(c) {
		return nil, ErrClaimNotSchedulable
	}

	// check if already reserved
	if r, err := s.findAlreadyReserved(ctx, c); err != nil || r != nil {
		return r, err
	}

	// start scheduling
	rr, err := s.listFreeRunners(ctx)
	if err != nil {
		return nil, err
	}

	// no hosts, can not schedule
	if len(rr) == 0 {
		return nil, ErrNoAvailableRunner
	}

	// reserve Runner
	r, err := s.findBestRunner(ctx, c, rr)
	if err != nil {
		return nil, err
	}

	s.setInUseBy(c, r)
	if err := s.Update(ctx, r); err != nil {
		return nil, err
	}

	return r, nil
}

func (s *Scheduler) setInUseBy(c maykonfluxcidevv1alpha1.Claim, r *maykonfluxcidevv1alpha1.Runner) {
	runner.SetInUseBy(r, c)
	controllerutil.AddFinalizer(r, constants.ClaimControllerFinalizer)
}

func (s *Scheduler) findBestRunner(
	_ context.Context,
	c maykonfluxcidevv1alpha1.Claim,
	rr []maykonfluxcidevv1alpha1.Runner,
) (*maykonfluxcidevv1alpha1.Runner, error) {
	f := c.Spec.Flavor
	for _, r := range rr {
		if _, ok := r.Spec.Resources[v1.ResourceName(f)]; ok {
			return &r, nil
		}
	}
	return nil, fmt.Errorf("no runner available for flavor %v", f)
}

func (s *Scheduler) listFreeRunners(ctx context.Context) ([]maykonfluxcidevv1alpha1.Runner, error) {
	// retrieve all Runners
	rr := maykonfluxcidevv1alpha1.RunnerList{}
	if err := s.List(ctx, &rr,
		client.InNamespace(s.namespace),
		client.MatchingFields{
			runner.FieldSpecInUseBy:   runner.FieldSpecInUseByValueNotReserved,
			runner.FieldIsStatusReady: "true",
		},
	); err != nil {
		return nil, err
	}

	// return free hosts
	return rr.Items, nil
}

func (s *Scheduler) findAlreadyReserved(
	ctx context.Context,
	c maykonfluxcidevv1alpha1.Claim,
) (*maykonfluxcidevv1alpha1.Runner, error) {
	// retrieve all Runners
	rr := maykonfluxcidevv1alpha1.RunnerList{}
	if err := s.List(ctx, &rr,
		client.InNamespace(s.namespace),
		client.MatchingFields{
			runner.FieldSpecInUseBy: runner.FieldSpecInUseByValueReserved,
		},
	); err != nil {
		return nil, err
	}

	// return free hosts
	for _, r := range rr.Items {
		if runner.IsInUseBy(r, c) {
			return &r, nil
		}
	}
	return nil, nil
}
