/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package provisioner

import (
	"github.com/prometheus/client_golang/prometheus"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
)

var (
	runnersCreated = prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: "may",
		Subsystem: "runner",
		Name:      "created",
		Help:      "Total number of runners created during host reconciliation",
	})

	runnerCleaningFailed = prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: "may",
		Subsystem: "runner",
		Name:      "cleaning_failed",
		Help:      "Total number of runners whose cleanup hooks failed",
	})
)

func init() {
	metrics.Registry.MustRegister(runnersCreated, runnerCleaningFailed)
}
