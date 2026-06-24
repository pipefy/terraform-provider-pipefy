// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

// Package pipegql holds the GraphQL field selection and the pure mapping shared
// by the pipefy_pipe resource and data source, so their reads stay in step.
package pipegql

const Selection = "id name public icon color " +
	"only_admin_can_remove_cards only_assignees_can_edit_cards " +
	"expiration_time_by_unit expiration_unit startFormPhaseId " +
	"preferences { inboxEmailEnabled mainTabViews }"

type Preferences struct {
	InboxEmailEnabled *bool    `json:"inboxEmailEnabled"`
	MainTabViews      []string `json:"mainTabViews"`
}

type Payload struct {
	Id                        string       `json:"id"`
	Name                      string       `json:"name"`
	Public                    *bool        `json:"public"`
	Icon                      *string      `json:"icon"`
	Color                     *string      `json:"color"`
	OnlyAdminCanRemoveCards   *bool        `json:"only_admin_can_remove_cards"`
	OnlyAssigneesCanEditCards *bool        `json:"only_assignees_can_edit_cards"`
	ExpirationTimeByUnit      *int64       `json:"expiration_time_by_unit"`
	ExpirationUnit            *int64       `json:"expiration_unit"`
	StartFormPhaseId          string       `json:"startFormPhaseId"`
	Preferences               *Preferences `json:"preferences"`
}

const (
	UnitMinutes = "minutes"
	UnitHours   = "hours"
	UnitDays    = "days"
)

var UnitNames = []string{UnitMinutes, UnitHours, UnitDays}

func UnitNameToSeconds(name string) (int64, bool) {
	switch name {
	case UnitMinutes:
		return 60, true
	case UnitHours:
		return 3600, true
	case UnitDays:
		return 86400, true
	}
	return 0, false
}

func UnitSecondsToName(seconds int64) (string, bool) {
	switch seconds {
	case 60:
		return UnitMinutes, true
	case 3600:
		return UnitHours, true
	case 86400:
		return UnitDays, true
	}
	return "", false
}

func ValidDuration(unit string, count int64) bool {
	if count < 1 {
		return false
	}
	switch unit {
	case UnitMinutes:
		return count <= 59
	case UnitHours:
		return count <= 23
	case UnitDays:
		return true
	}
	return false
}

func (p Payload) SLA() (count int64, unit string, ok bool) {
	if p.ExpirationUnit == nil || p.ExpirationTimeByUnit == nil {
		return 0, "", false
	}
	name, ok := UnitSecondsToName(*p.ExpirationUnit)
	if !ok {
		return 0, "", false
	}
	return *p.ExpirationTimeByUnit, name, true
}
