package domain

import "time"

type Dashboard struct {
	GeneratedAt time.Time        `json:"generated_at"`
	Summary     DashboardSummary `json:"summary"`
	Alerts      []Alert          `json:"alerts"`
}

type DashboardSummary struct {
	ClassroomsTotal   int `json:"classrooms_total"`
	ClassroomsReady   int `json:"classrooms_ready"`
	ClassroomsActive  int `json:"classrooms_active"`
	SeatsTotal        int `json:"seats_total"`
	SeatsReady        int `json:"seats_ready"`
	ThinClientsOnline int `json:"thin_clients_online"`
	DesktopsRunning   int `json:"desktops_running"`
	OperationsRunning int `json:"operations_running"`
	OperationsFailed  int `json:"operations_failed"`
}

type Alert struct {
	ID          string `json:"id"`
	Severity    string `json:"severity"`
	Title       string `json:"title"`
	Description string `json:"description"`
	ResourceID  string `json:"resource_id,omitempty"`
}
