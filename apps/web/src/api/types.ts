export type UUID = string;

export interface User {
  id: UUID;
  email: string;
  display_name?: string | null;
  created_at: string;
  service_account?: { id: UUID; organization_id: UUID } | null;
  /** True when registered for open signup but not yet approved by an admin. */
  signup_pending?: boolean;
  signup_request?: {
    id: UUID;
    organization_id: UUID;
    status: string;
  } | null;
}

export interface Organization {
  id: UUID;
  name: string;
  slug: string;
  datasets_enabled?: boolean;
  open_signups_enabled?: boolean;
  created_at: string;
  /** BYO public app origin for ops notes (https://…). */
  intended_public_url?: string | null;
  /** When set, API CORS and preview base URL prefer this origin at runtime (from “Sync server runtime” on save). */
  public_origin_override?: string | null;
}

/** GET /api/v1/organizations */
export interface OrganizationsListResponse {
  organizations: Organization[];
  /** When false (an org already exists in the DB), creating another workspace is blocked. */
  can_create_organization?: boolean;
}

export interface OrgFeatures {
  datasets_enabled: boolean;
  open_signups_enabled: boolean;
}

export interface Project {
  id: UUID;
  organization_id: UUID;
  name: string;
  description: string;
  created_at: string;
}

export interface Board {
  id: UUID;
  project_id: UUID;
  name: string;
  created_at?: string;
}

export interface ChatRoom {
  id: UUID;
  project_id: UUID;
  name: string;
  created_at: string;
}

export interface SpaceFile {
  id: UUID;
  project_id: UUID;
  name: string;
  created_at: string;
}

export type FileNodeKind = "folder" | "file";

export interface FileNode {
  id: UUID;
  project_id: UUID;
  parent_id?: UUID | null;
  kind: FileNodeKind;
  name: string;
  mime_type?: string | null;
  size_bytes?: number | null;
  storage_key?: string | null;
  checksum_sha256?: string | null;
  created_by: UUID;
  created_at: string;
  updated_at: string;
  deleted_at?: string | null;
}

/** Optional structured payload on assistant/system messages (e.g. Cloud Agent run cards). */
export interface ChatMessageAgentRunMeta {
  provider: string;
  external_id: string;
  status: string;
  url?: string;
  display_name?: string;
}

/** OpenRouter staff: links chat reply to pending file edit proposals for inline review. */
export interface ChatMessageFileEditProposalRef {
  proposal_id: UUID;
  node_id: UUID;
  file_name?: string;
}

export interface ChatMessage {
  id: UUID;
  chat_room_id: UUID;
  project_id: UUID;
  author_user_id?: UUID | null;
  content: string;
  /** Server JSON; may include `ai_agent_run` for provider runs. */
  metadata?: {
    ai_agent_run?: ChatMessageAgentRunMeta;
    file_edit_proposals?: ChatMessageFileEditProposalRef[];
  } | null;
  created_at: string;
  updated_at: string;
  edited_at?: string | null;
  deleted_at?: string | null;
}

export interface ChatMessageReaction {
  message_id: UUID;
  user_id: UUID;
  emoji: string;
  created_at: string;
}

export interface ChatMessageAttachment {
  id: UUID;
  message_id: UUID;
  name: string;
  mime: string;
  size_bytes: number;
  url: string;
  created_at: string;
}

export interface OrgMember {
  organization_id: UUID;
  user_id: UUID;
  role: "admin" | "member";
}

export type ServiceAccountProvider = "openrouter" | "cursor";

export interface OrgMemberWithUser {
  organization_id: UUID;
  user_id: UUID;
  role: "admin" | "member";
  email: string;
  display_name?: string | null;
  last_seen_at: string;
  is_service_account?: boolean;
  /** When the member is an AI staff service account */
  service_account_provider?: ServiceAccountProvider | null;
  openrouter_model?: string | null;
  cursor_default_repo_url?: string | null;
}

export interface ServiceAccount {
  id: UUID;
  organization_id: UUID;
  user_id: UUID;
  name: string;
  created_by: UUID;
  created_at: string;
  provider: ServiceAccountProvider;
  openrouter_model?: string | null;
  cursor_default_repo_url?: string | null;
  cursor_default_ref?: string | null;
}

export interface ServiceAccountProfileVersion {
  id: UUID;
  service_account_id: UUID;
  version: number;
  content_md: string;
  created_by: UUID;
  created_at: string;
}

export type FileEditProposalStatus = "pending" | "accepted" | "rejected";

export interface FileEditProposal {
  id: UUID;
  organization_id: UUID;
  space_id: UUID;
  node_id: UUID;
  author_user_id: UUID;
  base_content_sha256: string;
  /** Snapshot of file when the proposal was created; used for diff after accept. */
  base_content?: string | null;
  proposed_content: string;
  status: FileEditProposalStatus;
  created_at: string;
  resolved_at?: string | null;
  resolved_by?: UUID | null;
}

export interface Role {
  id: UUID;
  organization_id: UUID;
  name: string;
  is_system: boolean;
  created_at: string;
  updated_at: string;
}

export interface RoleWithPermissions extends Role {
  permissions: string[];
}

export interface BoardColumn {
  id: UUID;
  board_id: UUID;
  name: string;
  position: number;
}

export interface Task {
  id: UUID;
  /** Space id (API `space_id`; some older code referred to this as project_id). */
  space_id: UUID;
  board_id: UUID;
  column_id: UUID;
  title: string;
  description: string;
  assignee_user_id?: UUID | null;
  due_at?: string | null;
  deliverable_required: boolean;
  deliverable_instructions: string;
  position: number;
  version: number;
  created_at: string;
  updated_at: string;
}

export interface TaskMessage {
  id: UUID;
  space_id: UUID;
  task_id: UUID;
  author_user_id?: UUID | null;
  content: string;
  created_at: string;
}

export interface TaskDeliverableFile {
  task_id: UUID;
  file_node_id: UUID;
  added_by: UUID;
  created_at: string;
  file_name: string;
  mime_type?: string | null;
  size_bytes?: number | null;
}

export interface MyAssignedTask extends Task {
  organization_id: UUID;
  space_name: string;
}

/** GET /organizations/:orgId/peek/ai-activity */
export interface PeekAIActivityEntry {
  id: UUID;
  organization_id: UUID;
  space_id: UUID;
  chat_room_id: UUID;
  source_message_id: UUID;
  ai_user_id: UUID;
  requested_by_user_id: UUID;
  response_message_id?: UUID | null;
  created_at: string;
  responded_at?: string | null;
  space_name: string;
  chat_room_name: string;
}

/** Structured log (OpenRouter trace, Cursor conversation, file proposals). */
export type PeekAIRunDetail = Record<string, unknown>;

/** GET /organizations/:orgId/peek/ai-activity/runs/:replyId */
export interface PeekAIRunDetailResponse {
  reply: PeekAIActivityEntry & { run_detail?: PeekAIRunDetail | null };
}

/** IDE AI panel: Ask = read-only, Plan = read-only planning tone, Agent = full tools including proposals */
export type AgentChatMode = "ask" | "plan" | "agent";

export interface TokenResponse {
  access_token: string;
  refresh_token: string;
  token_type: string;
  expires_in: number;
  /** Present on register: true when staff signup awaits admin approval. */
  signup_pending?: boolean;
}

export interface SignupRequestRow {
  id: UUID;
  organization_id: UUID;
  user_id: UUID;
  status: string;
  created_at: string;
  resolved_at?: string | null;
  resolved_by_user_id?: UUID | null;
  email: string;
  display_name?: string | null;
}

export interface RealtimeEnvelope {
  type: string;
  organization_id: UUID;
  project_id?: UUID;
  payload: unknown;
}

export interface Notification {
  id: UUID;
  organization_id: UUID;
  user_id: UUID;
  type: string;
  payload: unknown;
  created_at: string;
  read_at?: string | null;
}
