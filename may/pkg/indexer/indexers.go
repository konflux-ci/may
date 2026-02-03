package indexer

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	maykonfluxcidevv1alpha1 "github.com/konflux-ci/may/api/v1alpha1"
	"github.com/konflux-ci/may/pkg/claim"
	"github.com/konflux-ci/may/pkg/runner"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func SetupFieldIndexers(ctx context.Context, mgr ctrl.Manager, setupLog logr.Logger) error {
	if err := mgr.GetFieldIndexer().
		IndexField(
			ctx,
			&maykonfluxcidevv1alpha1.Runner{},
			runner.FieldStatusReady,
			func(o client.Object) []string {
				r, ok := o.(*maykonfluxcidevv1alpha1.Runner)
				if !ok {
					panic("Runner Indexer: can not cast obj to runner")
				}

				if cr := runner.StatusConditionReadyReason(*r); cr != "" {
					return []string{cr}
				}
				return []string{}
			},
		); err != nil {
		setupLog.Error(err, fmt.Sprintf("unable to set up Runner Indexer for `%s`", runner.FieldStatusReady))
		return err
	}
	if err := mgr.GetFieldIndexer().
		IndexField(
			ctx,
			&maykonfluxcidevv1alpha1.Runner{},
			runner.FieldIsStatusReady,
			func(o client.Object) []string {
				r, ok := o.(*maykonfluxcidevv1alpha1.Runner)
				if !ok {
					panic("Runner Indexer: can not cast obj to runner")
				}

				if runner.IsReady(*r) {
					return []string{"true"}
				}
				return []string{"false"}
			},
		); err != nil {
		setupLog.Error(err, fmt.Sprintf("unable to set up Runner Indexer for `%s`", runner.FieldStatusReady))
		return err
	}
	if err := mgr.GetFieldIndexer().
		IndexField(
			ctx,
			&maykonfluxcidevv1alpha1.Runner{},
			runner.FieldSpecInUseBy,
			func(o client.Object) []string {
				r, ok := o.(*maykonfluxcidevv1alpha1.Runner)
				if !ok {
					panic("Runner Indexer: can not cast obj to runner")
				}

				if runner.IsReserved(*r) {
					return []string{runner.FieldSpecInUseByValueReserved}
				}
				return []string{runner.FieldSpecInUseByValueNotReserved}
			},
		); err != nil {
		setupLog.Error(err, fmt.Sprintf("unable to set up Runner Indexer for `%s`", runner.FieldSpecInUseBy))
		return err
	}
	if err := mgr.GetFieldIndexer().
		IndexField(
			ctx,
			&maykonfluxcidevv1alpha1.Claim{},
			claim.FieldStatusConditionClaimed,
			func(o client.Object) []string {
				c, ok := o.(*maykonfluxcidevv1alpha1.Claim)
				if !ok {
					panic("Runner Indexer: can not cast obj to Claim")
				}
				return []string{claim.ClaimedConditionReason(*c)}
			},
		); err != nil {
		setupLog.Error(err, fmt.Sprintf("unable to set up Claim Indexer for `%s`", claim.FieldStatusConditionClaimed))
		return err
	}
	if err := mgr.GetFieldIndexer().
		IndexField(
			ctx,
			&maykonfluxcidevv1alpha1.Runner{},
			runner.FieldSpecInUseByName,
			func(o client.Object) []string {
				r, ok := o.(*maykonfluxcidevv1alpha1.Runner)
				if !ok {
					panic("Runner Indexer: can not cast obj to runner")
				}

				if !runner.IsReserved(*r) {
					return []string{}
				}
				return []string{r.Spec.InUseBy.Name}
			},
		); err != nil {
		setupLog.Error(err, fmt.Sprintf("unable to set up Runner Indexer for `%s`", runner.FieldSpecInUseByName))
		return err
	}
	if err := mgr.GetFieldIndexer().
		IndexField(
			ctx,
			&maykonfluxcidevv1alpha1.Runner{},
			runner.FieldSpecInUseByNamespace,
			func(o client.Object) []string {
				r, ok := o.(*maykonfluxcidevv1alpha1.Runner)
				if !ok {
					panic("Runner Indexer: can not cast obj to runner")
				}

				if !runner.IsReserved(*r) {
					return []string{}
				}
				return []string{r.Spec.InUseBy.Namespace}
			},
		); err != nil {
		setupLog.Error(err, fmt.Sprintf("unable to set up Runner Indexer for `%s`", runner.FieldSpecInUseByNamespace))
		return err
	}
	return nil
}
