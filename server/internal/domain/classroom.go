package domain

import "time"

type ClassroomStatus string

const (
	ClassroomReady    ClassroomStatus = "READY"
	ClassroomActive   ClassroomStatus = "ACTIVE"
	ClassroomDegraded ClassroomStatus = "DEGRADED"
	ClassroomOffline  ClassroomStatus = "OFFLINE"
)

type PowerState string

const (
	PowerRunning PowerState = "RUNNING"
	PowerStopped PowerState = "STOPPED"
	PowerUnknown PowerState = "UNKNOWN"
)

type Classroom struct {
	ID              string          `json:"id"`
	OrganizationID  string          `json:"organization_id"`
	Name            string          `json:"name"`
	Site            string          `json:"site"`
	Status          ClassroomStatus `json:"status"`
	Timezone        string          `json:"timezone"`
	TemplateName    string          `json:"template_name"`
	TemplateVersion string          `json:"template_version"`
	ActiveSession   *string         `json:"active_session"`
	Seats           []Seat          `json:"seats,omitempty"`
	ResourceVersion int64           `json:"resource_version"`
	UpdatedAt       time.Time       `json:"updated_at"`
}

type Seat struct {
	ID             string          `json:"id"`
	Label          string          `json:"label"`
	Terminal       *ThinClient     `json:"terminal"`
	Desktop        *VirtualDesktop `json:"desktop"`
	OperationState ItemStatus      `json:"operation_state"`
	UserName       *string         `json:"user_name"`
}

type ThinClient struct {
	ID           string     `json:"id"`
	Name         string     `json:"name"`
	Online       bool       `json:"online"`
	IPAddress    string     `json:"ip_address"`
	Architecture string     `json:"architecture"`
	AgentVersion string     `json:"agent_version"`
	LastSeenAt   *time.Time `json:"last_seen_at"`
}

type VirtualDesktop struct {
	ID               string     `json:"id"`
	Name             string     `json:"name"`
	ClusterID        string     `json:"cluster_id"`
	PVEVMID          int        `json:"pve_vmid"`
	DesiredState     PowerState `json:"desired_state"`
	ObservedState    PowerState `json:"observed_state"`
	TemplateVersion  string     `json:"template_version"`
	BaselineSnapshot string     `json:"baseline_snapshot_name,omitempty"`
	GuestAgentReady  bool       `json:"guest_agent_ready"`
	LastReconciledAt *time.Time `json:"last_reconciled_at"`
	ConfigHash       string     `json:"config_hash"`
}

type ClassroomSummary struct {
	ID                string          `json:"id"`
	Name              string          `json:"name"`
	Site              string          `json:"site"`
	Status            ClassroomStatus `json:"status"`
	Timezone          string          `json:"timezone"`
	SeatsTotal        int             `json:"seats_total"`
	SeatsReady        int             `json:"seats_ready"`
	ThinClientsOnline int             `json:"thin_clients_online"`
	DesktopsRunning   int             `json:"desktops_running"`
	TemplateName      string          `json:"template_name"`
	TemplateVersion   string          `json:"template_version"`
	ActiveSession     *string         `json:"active_session"`
	UpdatedAt         time.Time       `json:"updated_at"`
}

func SummarizeClassroom(classroom Classroom) ClassroomSummary {
	summary := ClassroomSummary{
		ID:              classroom.ID,
		Name:            classroom.Name,
		Site:            classroom.Site,
		Status:          classroom.Status,
		Timezone:        classroom.Timezone,
		SeatsTotal:      len(classroom.Seats),
		TemplateName:    classroom.TemplateName,
		TemplateVersion: classroom.TemplateVersion,
		ActiveSession:   classroom.ActiveSession,
		UpdatedAt:       classroom.UpdatedAt,
	}
	for _, seat := range classroom.Seats {
		terminalReady := seat.Terminal != nil && seat.Terminal.Online
		desktopReady := seat.Desktop != nil && seat.Desktop.ObservedState == PowerRunning && seat.Desktop.GuestAgentReady
		if terminalReady {
			summary.ThinClientsOnline++
		}
		if seat.Desktop != nil && seat.Desktop.ObservedState == PowerRunning {
			summary.DesktopsRunning++
		}
		if terminalReady && desktopReady && seat.OperationState != ItemFailed && seat.OperationState != ItemUnknown {
			summary.SeatsReady++
		}
	}
	return summary
}
