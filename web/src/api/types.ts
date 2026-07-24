export type ClassroomStatus = 'READY' | 'ACTIVE' | 'DEGRADED' | 'OFFLINE';
export type PowerState =
  | 'RUNNING'
  | 'STOPPED'
  | 'STARTING'
  | 'STOPPING'
  | 'ERROR'
  | 'UNKNOWN';
export type OperationStatus =
  | 'QUEUED'
  | 'VALIDATING'
  | 'RUNNING'
  | 'WAITING_PVE'
  | 'VERIFYING'
  | 'SUCCEEDED'
  | 'PARTIALLY_SUCCEEDED'
  | 'FAILED'
  | 'CANCEL_REQUESTED'
  | 'CANCELLED';
export type OperationType = 'PRECHECK' | 'START' | 'SHUTDOWN' | 'RESTORE';

export interface DashboardSummary {
  classrooms_total: number;
  classrooms_ready: number;
  classrooms_active: number;
  seats_total: number;
  seats_ready: number;
  thin_clients_online: number;
  desktops_running: number;
  operations_running: number;
  operations_failed: number;
}

export interface DashboardAlert {
  id: string;
  severity: 'info' | 'warning' | 'critical';
  title: string;
  description: string;
  resource_id?: string;
  resource_type?: string;
  created_at?: string;
}

export interface Dashboard {
  generated_at: string;
  summary: DashboardSummary;
  alerts: DashboardAlert[];
}

export interface ActiveSession {
  id: string;
  name: string;
  status: string;
  starts_at: string;
  ends_at: string;
}

export interface ClassroomSummary {
  id: string;
  name: string;
  site: string;
  status: ClassroomStatus;
  timezone: string;
  seats_total: number;
  seats_ready: number;
  thin_clients_online: number;
  desktops_running: number;
  template_name: string;
  template_version: string;
  active_session: ActiveSession | string | null;
  updated_at: string;
}

export interface ThinClient {
  id: string;
  name: string;
  online?: boolean;
  status?: 'online' | 'offline' | 'stale';
  ip_address?: string;
  architecture?: string;
  arch?: string;
  agent_version?: string;
  last_seen_at: string | null;
}

export interface VirtualDesktop {
  id: string;
  name: string;
  cluster_id?: string;
  pve_vmid?: number;
  vmid?: number;
  desired_state: PowerState;
  observed_state: PowerState;
  power_state?: PowerState;
  template_version: string;
  baseline_snapshot_name?: string;
  guest_agent_ready?: boolean;
  ip_address?: string;
  last_reconciled_at?: string | null;
  config_hash?: string;
}

export interface Seat {
  id: string;
  label: string;
  terminal: ThinClient | null;
  desktop: VirtualDesktop | null;
  operation_state: string;
  user_name: string | null;
}

export interface ClassroomDetail extends Omit<ClassroomSummary, 'seats_total' | 'seats_ready' | 'thin_clients_online' | 'desktops_running'> {
  organization_id?: string;
  seats_total?: number;
  seats_ready?: number;
  thin_clients_online?: number;
  desktops_running?: number;
  resource_version?: number;
  seats: Seat[];
}

export interface ClassroomList {
  items: ClassroomSummary[];
  total: number;
  generated_at?: string;
}

export interface OperationCounts {
  total: number;
  queued: number;
  running: number;
  succeeded: number;
  failed: number;
  skipped: number;
  unknown: number;
}

export interface OperationItem {
  id: string;
  operation_id?: string;
  seat_id: string;
  seat_label?: string;
  desktop_id?: string | null;
  cluster_id?: string;
  pve_vmid?: number;
  target_name?: string;
  snapshot_name?: string;
  status: string;
  phase?: string;
  upid?: string;
  error_code?: string;
  error_message?: string;
  message?: string;
  started_at?: string | null;
  completed_at?: string | null;
  updated_at?: string;
}

export interface Operation {
  id: string;
  classroom_id: string;
  classroom_name?: string;
  type: OperationType;
  status: OperationStatus;
  reason?: string;
  request_id?: string;
  counts: OperationCounts;
  items: OperationItem[];
  resource_version?: number;
  created_at: string;
  updated_at: string;
  started_at?: string | null;
  completed_at?: string | null;
}

export interface OperationList {
  items: Operation[];
  total: number;
  generated_at?: string;
}

export type OperationEventType = 'operation.snapshot' | 'operation.updated' | 'operation.item.updated';

export interface OperationProgress {
  total: number;
  completed: number;
  queued: number;
  running: number;
  succeeded: number;
  failed: number;
  skipped: number;
  unknown: number;
}

export interface OperationItemEventState {
  item_id: string;
  seat_id: string;
  seat_label: string;
  target_name: string;
  status: string;
  error_code?: string;
  message?: string;
  updated_at: string;
}

export interface OperationEvent {
  event_type: OperationEventType;
  operation_id: string;
  item_id?: string;
  sequence: number;
  timestamp: string;
  operation_status: OperationStatus;
  item_status?: string;
  seat_id?: string;
  seat_label?: string;
  target_name?: string;
  error_code?: string;
  message?: string;
  item_updated_at?: string;
  progress: OperationProgress;
  resource_version: number;
  reset?: boolean;
  items?: OperationItemEventState[];
}

export interface CreateClassroomOperationInput {
  type: OperationType;
  seat_ids?: string[];
  reason?: string;
  confirmed?: boolean;
}

export interface ProblemDetails {
  error_code?: string;
  message?: string;
  request_id?: string;
  field_errors?: Record<string, string>;
}
