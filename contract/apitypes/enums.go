package apitypes

// The closed sets of words the API answers with beside the statuses in contract/status. Each is
// a named string so the TypeScript the dashboard builds against narrows to exactly these values
// (enums in typescript.go lists them for the generator).

// Role is a person's role in a business. Support is what an operator's support access confers
// for an hour; no membership holds it.
type Role string

const (
	RoleOwner     Role = "owner"
	RoleAdmin     Role = "admin"
	RoleDeveloper Role = "developer"
	RoleBilling   Role = "billing"
	RoleMember    Role = "member"
	RoleSupport   Role = "support"
)

// MemberKind is whether a person is a member of a business or a guest in it.
type MemberKind string

const (
	KindMember MemberKind = "member"
	KindGuest  MemberKind = "guest"
)

// MemberStatus is whether a membership waits on its invitation.
type MemberStatus string

const (
	MemberInvited MemberStatus = "invited"
	MemberActive  MemberStatus = "active"
)

// AppState is the human word for how an app is doing (DASHBOARD.md §7).
type AppState string

const (
	AppNew      AppState = "new"
	AppRunning  AppState = "running"
	AppSleeping AppState = "sleeping"
	AppStarting AppState = "starting"
	AppBroken   AppState = "broken"
	AppBlocked  AppState = "blocked"
)

// EnvironmentStatus is whether an environment is up or asleep.
type EnvironmentStatus string

const (
	EnvironmentActive   EnvironmentStatus = "active"
	EnvironmentSleeping EnvironmentStatus = "sleeping"
)

// BuildStatus is an image build's status.
type BuildStatus string

const (
	BuildQueued    BuildStatus = "queued"
	BuildRunning   BuildStatus = "running"
	BuildSucceeded BuildStatus = "succeeded"
	BuildFailed    BuildStatus = "failed"
	BuildCancelled BuildStatus = "cancelled"
)

// FrozenReason is why a business is frozen (CONTROL-PLANE.md §6.15).
type FrozenReason string

const (
	FrozenPayment  FrozenReason = "payment"
	FrozenOperator FrozenReason = "operator"
	FrozenInactive FrozenReason = "inactive"
	FrozenAbuse    FrozenReason = "abuse"
)

// ShreddingFor is why a business is being deleted.
type ShreddingFor string

const (
	ShreddingDeleted  ShreddingFor = "deleted"
	ShreddingPayment  ShreddingFor = "payment"
	ShreddingInactive ShreddingFor = "inactive"
	ShreddingOperator ShreddingFor = "operator"
	ShreddingAbuse    ShreddingFor = "abuse"
)

// SecretScope is whether a secret belongs to the business or to one app.
type SecretScope string

const (
	ScopeOrg SecretScope = "org"
	ScopeApp SecretScope = "app"
)

// SecretReader is what read a secret's value: a running container or a break-glass read.
type SecretReader string

const (
	ReaderContainer  SecretReader = "container"
	ReaderBreakglass SecretReader = "breakglass"
)

// SubjectKind is who a grant names.
type SubjectKind string

const (
	SubjectUser     SubjectKind = "user"
	SubjectGroup    SubjectKind = "group"
	SubjectEveryone SubjectKind = "everyone"
)

// Audience is whether a grant is for the team or for the business's own customers.
type Audience string

const (
	AudienceTeam     Audience = "team"
	AudienceCustomer Audience = "customer"
)

// DomainKind is where an app's hostname comes from: the platform's, a custom one, the business's
// own domain, or a former address that redirects (CONTROL-PLANE.md §6.5).
type DomainKind string

const (
	DomainPlatform DomainKind = "platform"
	DomainCustom   DomainKind = "custom"
	DomainBusiness DomainKind = "business"
	DomainFormer   DomainKind = "former"
)

// DomainStatus is where a custom domain is (CONTROL-PLANE.md §6.11): its records are not in
// place yet, it is verified and the edge has not presented its certificate yet, or it answers.
type DomainStatus string

const (
	DomainPendingDNS         DomainStatus = "pending_dns"
	DomainPendingCertificate DomainStatus = "pending_certificate"
	DomainActive             DomainStatus = "active"
)

// DomainAddedBy is who added a custom domain: the business's people and their agents, or the
// app itself with its service token.
type DomainAddedBy string

const (
	DomainAddedByTeam DomainAddedBy = "team"
	DomainAddedByApp  DomainAddedBy = "app"
)

// SignInMethod is how a session was signed in (CONTROL-PLANE.md §4.1).
type SignInMethod string

const (
	MethodPasskey  SignInMethod = "passkey"
	MethodPassword SignInMethod = "password"
	MethodCode     SignInMethod = "code"
	MethodSSO      SignInMethod = "sso"
	MethodTest     SignInMethod = "test"
)

// ActorKind is who did what an audit entry records.
type ActorKind string

const (
	ActorUser     ActorKind = "user"
	ActorAgent    ActorKind = "agent"
	ActorSystem   ActorKind = "system"
	ActorOperator ActorKind = "operator"
	ActorService  ActorKind = "service"
)

// DeviceStatus is where a device code stands (CONTROL-PLANE.md §4.5).
type DeviceStatus string

const (
	DevicePending  DeviceStatus = "pending"
	DeviceApproved DeviceStatus = "approved"
	DeviceDenied   DeviceStatus = "denied"
	DeviceExpired  DeviceStatus = "expired"
)
