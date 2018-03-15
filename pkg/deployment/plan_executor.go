//
// DISCLAIMER
//
// Copyright 2018 ArangoDB GmbH, Cologne, Germany
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//
// Copyright holder is ArangoDB GmbH, Cologne, Germany
//
// Author Ewout Prangsma
//

package deployment

import (
	"context"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	api "github.com/arangodb/kube-arangodb/pkg/apis/deployment/v1alpha"
	"github.com/rs/zerolog"
)

// executePlan tries to execute the plan as far as possible.
// Returns true when it has to be called again soon.
// False otherwise.
func (d *Deployment) executePlan(ctx context.Context) (bool, error) {
	log := d.deps.Log

	for {
		if len(d.status.Plan) == 0 {
			// No plan exists, nothing to be done
			return false, nil
		}

		// Take first action
		planAction := d.status.Plan[0]
		log := log.With().
			Int("plan-len", len(d.status.Plan)).
			Str("action-id", planAction.ID).
			Str("action-type", string(planAction.Type)).
			Str("group", planAction.Group.AsRole()).
			Str("member-id", planAction.MemberID).
			Logger()
		action := d.createAction(ctx, log, planAction)
		if planAction.StartTime.IsZero() {
			// Not started yet
			ready, err := action.Start(ctx)
			if err != nil {
				log.Debug().Err(err).
					Msg("Failed to start action")
				return false, maskAny(err)
			}
			if ready {
				// Remove action from list
				d.status.Plan = d.status.Plan[1:]
			} else {
				// Mark start time
				now := metav1.Now()
				d.status.Plan[0].StartTime = &now
			}
			// Save plan update
			if err := d.updateCRStatus(true); err != nil {
				log.Debug().Err(err).Msg("Failed to update CR status")
				return false, maskAny(err)
			}
			log.Debug().Bool("ready", ready).Msg("Action Start completed")
			if !ready {
				// We need to check back soon
				return true, nil
			}
			// Continue with next action
		} else {
			// First action of plan has been started, check its progress
			ready, err := action.CheckProgress(ctx)
			if err != nil {
				log.Debug().Err(err).Msg("Failed to check action progress")
				return false, maskAny(err)
			}
			if ready {
				// Remove action from list
				d.status.Plan = d.status.Plan[1:]
				// Save plan update
				if err := d.updateCRStatus(); err != nil {
					log.Debug().Err(err).Msg("Failed to update CR status")
					return false, maskAny(err)
				}
			}
			log.Debug().Bool("ready", ready).Msg("Action CheckProgress completed")
			if !ready {
				// Not ready check, come back soon
				return true, nil
			}
			// Continue with next action
		}
	}
}

// startAction performs the start of the given action
// Returns true if the action is completely finished, false in case
// the start time needs to be recorded and a ready condition needs to be checked.
func (d *Deployment) createAction(ctx context.Context, log zerolog.Logger, action api.Action) Action {
	actionCtx := NewActionContext(log, d)
	switch action.Type {
	case api.ActionTypeAddMember:
		return NewAddMemberAction(log, action, actionCtx)
	case api.ActionTypeRemoveMember:
		return NewRemoveMemberAction(log, action, actionCtx)
	case api.ActionTypeCleanOutMember:
		return NewCleanOutMemberAction(log, action, actionCtx)
	case api.ActionTypeShutdownMember:
		return NewShutdownMemberAction(log, action, actionCtx)
	case api.ActionTypeRotateMember:
		return NewRotateMemberAction(log, action, actionCtx)
	case api.ActionTypeWaitForMemberUp:
		return NewWaitForMemberUpAction(log, action, actionCtx)
	default:
		panic(fmt.Sprintf("Unknown action type '%s'", action.Type))
	}
}
