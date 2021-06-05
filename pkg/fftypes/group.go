// Copyright © 2021 Kaleido, Inc.
//
// SPDX-License-Identifier: Apache-2.0
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package fftypes

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"

	"github.com/kaleido-io/firefly/internal/i18n"
)

type Group struct {
	ID          *UUID    `json:"id"`
	Message     *UUID    `json:"message,omitempty"`
	Namespace   string   `json:"namespace,omitempty"`
	Description string   `json:"description,omitempty"`
	Ledger      *UUID    `json:"ledger,omitempty"`
	Hash        *Bytes32 `json:"hash,omitempty"`
	Created     *FFTime  `json:"created,omitempty"`
	Members     Members  `json:"members"`
}

type Members []*Member

type Member struct {
	Org  *UUID `json:"org,omitempty"`
	Node *UUID `json:"node,omitempty"`
}

type MemberInput struct {
	Org  string `json:"org,omitempty"`
	Node string `json:"node,omitempty"`
}

func (r *Members) Hash() *Bytes32 {
	b, _ := json.Marshal(&r)
	hash := Bytes32(sha256.Sum256(b))
	return &hash
}

func (group *Group) Validate(ctx context.Context, existing bool) (err error) {
	if err = ValidateFFNameField(ctx, group.Namespace, "namespace"); err != nil {
		return err
	}
	if err = ValidateLength(ctx, group.Description, "description", 4096); err != nil {
		return err
	}
	if len(group.Members) == 0 {
		return i18n.NewError(ctx, i18n.MsgGroupMustHaveMembers)
	}
	dupCheck := make(map[string]bool)
	for i, r := range group.Members {
		if r.Org == nil {
			return i18n.NewError(ctx, i18n.MsgEmptyMemberOrg, i)
		}
		if r.Node == nil {
			return i18n.NewError(ctx, i18n.MsgEmptyMemberNode, i)
		}
		key := fmt.Sprintf("%s:%s", r.Org, r.Node)
		if dupCheck[key] {
			return i18n.NewError(ctx, i18n.MsgDuplicateMember, i)
		}
		dupCheck[key] = true
	}
	if existing {
		if group.ID == nil {
			return i18n.NewError(ctx, i18n.MsgNilID)
		}
	}
	return nil
}

func (group *Group) Seal() {
	group.Hash = group.Members.Hash()
}

func (group *Group) Topic() string {
	return fmt.Sprintf("ff-grp-%s", group.ID)
}

func (group *Group) SetBroadcastMessage(msgID *UUID) {
	group.Message = msgID
}
