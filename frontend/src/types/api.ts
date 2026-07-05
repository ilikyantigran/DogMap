// API contract types — mirror the REST edge shapes in Docs/02-Backend.md.
// These map 1:1 to the backend proto messages. Keep them in sync with that doc;
// it is the source of truth for the wire format.

// ---- Enums (Docs/02-Backend.md "Enums") ----

export type ObjectType = 'PARK' | 'DOG_PARK' | 'DOG_BEACH'

export type PresenceAction = 'VISITING' | 'NOT_VISITING'

export type FriendStatus =
  | 'NONE'
  | 'PENDING_OUT'
  | 'PENDING_IN'
  | 'FRIENDS'
  | 'BLOCKED'

export type PetSex = 'M' | 'F'

// ---- Envelope ----
// Every response carries code (int) + message (string) on error paths.
export interface ApiEnvelope {
  code: number
  message: string
}

// ---- Pets / Users ----

export interface Pet {
  breed: string
  name: string
  sex: PetSex
  is_castrated: boolean
  age: number
}

// Full profile (self or friend). Non-friends receive a reduced shape where
// email / phone / current_object_id are omitted (see privacy rules).
export interface UserInfo {
  user_id: string
  login: string
  name: string
  surname: string
  email?: string // friends-only PII
  phone?: string // friends-only PII
  pets: Pet[]
  on_walk: boolean
  current_object_id?: string | null // may be omitted for non-friends
  friend_status: FriendStatus
}

// ---- Auth ----

export interface RegisterRequest {
  login: string
  email: string
  password: string
}

export interface RegisterResponse extends ApiEnvelope {
  user_id: string
}

export interface LoginRequest {
  // Need (login OR email) AND password.
  login?: string
  email?: string
  password: string
}

export interface LoginResponse extends ApiEnvelope {
  token: string
  user_id: string
}

// ---- Profiles ----

export interface EditUserRequest {
  // Acting user derived from token; NO user_id in body.
  // login/email are NOT editable here (login immutable, email is a separate flow).
  name: string
  surname: string
  phone: string
  pets: Pet[]
}

export interface GetUserInfoRequest {
  user_id_target: string
}

// ---- Friends ----

export interface FriendSummary {
  user_id: string
  login: string
  on_walk: boolean
  current_object_id?: string | null
}

export interface IncomingRequest {
  from_user_id: string
  from_login: string
  friend_request_id: string
}

export interface OutgoingRequest {
  to_user_id: string
  friend_request_id: string
}

export interface ListFriendsResponse extends ApiEnvelope {
  friends: FriendSummary[]
  incoming_requests: IncomingRequest[]
  outgoing_requests: OutgoingRequest[]
}

export interface SendFriendRequestRequest {
  user_id_target: string
}

export interface SendFriendRequestResponse extends ApiEnvelope {
  friend_request_id: string
}

export interface SendFriendResponseRequest {
  friend_request_id: string
  resolution: boolean // true = accept, false = decline
}

export interface TargetUserRequest {
  user_id_target: string
}

// ---- Map ----

export interface MapObject {
  id: string
  object_type: ObjectType
  longitude: number
  latitude: number
  visitor_count: number
  // friend_ids_here is computed for the caller only. Raw visitor lists are
  // NEVER sent to the client (privacy). Absent/[] when no friends are present.
  friend_ids_here: string[]
  // viewer_visiting: true when the CALLER currently holds presence here. Lets the
  // client show the correct toggle state and avoid re-marking after a refresh.
  viewer_visiting: boolean
  // Human-readable object name (may be '' for unnamed OSM features).
  name: string
}

// One friend currently on a walk and where they are. Returned by FriendsPresence
// (POST /v1/map/friends-presence). Only friends holding a live presence key appear.
export interface FriendPresence {
  user_id: string
  object_id: string
  object_name: string
  latitude: number
  longitude: number
}

export interface FriendsPresenceResponse extends ApiEnvelope {
  friends: FriendPresence[]
}

export interface LoadMapRequest {
  longitude: number
  latitude: number
}

export interface LoadMapResponse extends ApiEnvelope {
  objects: MapObject[]
}

export interface GetMapObjectRequest {
  id: string
}

export interface ChangeMapObjectStatusRequest {
  // Acting user derived from token; NO user_id in body.
  id: string
  action: PresenceAction
}

// GetMapObject / ChangeMapObjectStatus return a single object shape. The backend
// wraps it in an envelope; the exact key is not pinned in the doc, so the map
// client tolerates both `{ ...MapObject }` and `{ object: MapObject }`.
export interface MapObjectResponse extends ApiEnvelope {
  object?: MapObject
}
