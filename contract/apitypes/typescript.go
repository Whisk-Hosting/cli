package apitypes

import (
	"encoding/json"
	"fmt"
	"reflect"
	"slices"
	"strings"
	"time"

	werrors "github.com/whisk-run/contract/errors"
	"github.com/whisk-run/contract/graph"
	"github.com/whisk-run/contract/status"
)

// TypeScript is the TypeScript the dashboard builds against: every type in this package, the
// statuses it carries from contract/status and the error body, written from the Go types by
// reflection so the two cannot disagree. typescript_test.go keeps the dashboard's copy current.
func TypeScript() (string, error) { return render(wire, enums) }

// typeParam stands for a generic type's parameter while the generic is rendered.
type typeParam struct{}

// generic is a generic type rendered once with typeParam for its parameter.
type generic struct {
	name, param string
	sample      any
}

// enumSet is a named string type and every value it takes. list names a TypeScript const array
// holding the values; the type is the union of its members.
type enumSet struct {
	name, list string
	values     []string
}

func enumOf[T ~string](name, list string, values ...T) (reflect.Type, enumSet) {
	out := make([]string, len(values))
	for i, v := range values {
		out[i] = string(v)
	}
	return reflect.TypeFor[T](), enumSet{name: name, list: list, values: out}
}

// enums is every named string type the wire types use. A type missing here is an error, so a new
// one is never written out as a bare string.
var enums = func() map[reflect.Type]enumSet {
	m := map[reflect.Type]enumSet{}
	add := func(t reflect.Type, e enumSet) { m[t] = e }
	add(enumOf("DeployStatus", "DEPLOY_STATUSES", status.Deploys()...))
	add(enumOf("OrgStatus", "ORG_STATUSES", status.Orgs()...))
	add(enumOf("AppStatus", "APP_STATUSES", status.Apps()...))
	add(enumOf("NodeStatus", "NODE_STATUSES", status.Nodes()...))
	add(enumOf("Role", "ROLES", RoleOwner, RoleAdmin, RoleDeveloper, RoleBilling, RoleMember, RoleSupport))
	add(enumOf("MemberKind", "MEMBER_KINDS", KindMember, KindGuest))
	add(enumOf("MemberStatus", "MEMBER_STATUSES", MemberInvited, MemberActive))
	add(enumOf("AppState", "APP_STATES", AppNew, AppRunning, AppSleeping, AppStarting, AppBroken, AppBlocked))
	add(enumOf("EnvironmentStatus", "ENVIRONMENT_STATUSES", EnvironmentActive, EnvironmentSleeping))
	add(enumOf("BuildStatus", "BUILD_STATUSES", BuildQueued, BuildRunning, BuildSucceeded, BuildFailed, BuildCancelled))
	add(enumOf("FrozenReason", "FROZEN_REASONS", FrozenPayment, FrozenOperator, FrozenInactive, FrozenAbuse))
	add(enumOf("ShreddingFor", "SHREDDING_REASONS", ShreddingDeleted, ShreddingPayment, ShreddingInactive, ShreddingOperator, ShreddingAbuse))
	add(enumOf("SecretScope", "SECRET_SCOPES", ScopeOrg, ScopeApp))
	add(enumOf("SecretReader", "SECRET_READERS", ReaderContainer, ReaderBreakglass))
	add(enumOf("SubjectKind", "SUBJECT_KINDS", SubjectUser, SubjectGroup, SubjectEveryone))
	add(enumOf("Audience", "AUDIENCES", AudienceTeam, AudienceCustomer))
	add(enumOf("DomainKind", "DOMAIN_KINDS", DomainPlatform, DomainCustom, DomainBusiness, DomainFormer))
	add(enumOf("DomainStatus", "DOMAIN_STATUSES", DomainPendingDNS, DomainPendingCertificate, DomainActive))
	add(enumOf("DomainAddedBy", "DOMAIN_ADDERS", DomainAddedByTeam, DomainAddedByApp))
	add(enumOf("SignInMethod", "SIGN_IN_METHODS", MethodPasskey, MethodPassword, MethodCode, MethodSSO, MethodTest))
	add(enumOf("ActorKind", "ACTOR_KINDS", ActorUser, ActorAgent, ActorSystem, ActorOperator, ActorService))
	add(enumOf("DeviceStatus", "DEVICE_STATUSES", DevicePending, DeviceApproved, DeviceDenied, DeviceExpired))
	add(enumOf("LicenceState", "LICENCE_STATES", LicenceMissing, LicenceInvalid, LicenceActive, LicenceEnding, LicenceExpired))
	add(enumOf("DeviceScope", "DEVICE_SCOPES", DeviceScopeApp, DeviceScopeOrg, DeviceScopePick, DeviceScopeNew))
	add(enumOf("LoginScope", "LOGIN_SCOPES", LoginScopeAccount, LoginScopeBusiness, LoginScopeApp))
	add(enumOf("HoldStatus", "HOLD_STATUSES", HoldHeld, HoldReleased, HoldRemoved))
	add(enumOf("HoldKind", "HOLD_KINDS", HoldPhishingPage, HoldBlocklist, HoldWebRisk, HoldEmailLink, HoldLinked))
	add(enumOf("AbuseKind", "ABUSE_KINDS", AbuseEgress, AbuseEmailThrottled, AbuseCPUPegged, AbuseRunGuard, AbuseDisposableSignup))
	add(enumOf("FeedbackKind", "FEEDBACK_KINDS", FeedbackBug, FeedbackDifficulty, FeedbackIdea, FeedbackPraise))
	add(enumOf("FeedbackStatus", "FEEDBACK_STATUSES", FeedbackOpen, FeedbackResolved))
	add(enumOf("FeedbackSender", "FEEDBACK_SENDERS", SenderUser, SenderAgent, SenderAnonymous))
	add(enumOf("Interval", "INTERVALS", Monthly, Yearly))
	add(enumOf("InvoiceStatus", "INVOICE_STATUSES", InvoiceDraft, InvoiceOpen, InvoicePaid, InvoiceUncollectible, InvoiceVoid))
	add(enumOf("PayKind", "PAY_KINDS", PaySubscribe, PayCard, PayInvoice))
	add(enumOf("PayInterval", "PAY_INTERVALS", PayMonthly, PayYearly, PayNoInterval))
	add(enumOf("PayStepKind", "PAY_STEP_KINDS", StepSetup, StepPayment, StepDone))
	add(enumOf("ClientBilling", "CLIENT_BILLINGS", ClientUnbilled, ClientTrialing, ClientPaying, ClientPastDue, ClientFrozen, ClientDeleting))
	add(enumOf("AgencyBlock", "AGENCY_BLOCKS", AgencyPending, AgencyInactive, AgencyIsClient, AgencyCanary, AgencyUnpaid))
	add(enumOf("PayoutAccount", "PAYOUT_ACCOUNTS", PayoutNone, PayoutPending, PayoutReady))
	add(enumOf("StripeSource", "STRIPE_SOURCES", StripeFromOperator, StripeFromEnvironment, StripeNone))
	add(enumOf("StripeMode", "STRIPE_MODES", StripeLive, StripeTest, StripeNoMode))
	add(enumOf("DriftKind", "DRIFT_KINDS", DriftCustomerMissing, DriftSubscriptionUnknown, DriftSubscriptionMissing, DriftSubscriptionPlan, DriftSubscriptionItems, DriftSubscriptionPaused, DriftInvoiceUnrecorded, DriftInvoiceStale))
	add(enumOf("NumberPeriod", "NUMBER_PERIODS", NumbersWeek, NumbersDay))
	add(enumOf("NumberChannel", "NUMBER_CHANNELS", ChannelServer, ChannelBrowser))
	add(enumOf("NumberInclude", "NUMBER_INCLUDES", IncludeBots, IncludeMalicious, IncludeWhiskUsers, IncludeOtherSites, IncludeAppPages, IncludeErrors))
	add(enumOf("NumberGroup", "NUMBER_GROUPS", GroupSite, GroupStart, GroupWhisk, GroupSearch))
	add(enumOf("NumberVerdict", "NUMBER_VERDICTS", VerdictEmpty, VerdictTooFew, VerdictSteady, VerdictSignal))
	add(enumOf("NumberSignal", "NUMBER_SIGNALS", SignalOutside, SignalRun, SignalNearLimit, SignalJump))
	add(enumOf("ApprovalStatus", "APPROVAL_STATUSES", ApprovalPending, ApprovalApproved, ApprovalRejected, ApprovalExpired))
	add(enumOf("AgentCategory", "AGENT_CATEGORIES", CategoryRead, CategoryOperate, CategoryChange, CategoryDestroy))
	add(enumOf("DemoKind", "DEMO_KINDS", HandoverDemo, HandoverClient))
	add(enumOf("NotificationChannel", "NOTIFICATION_CHANNELS", ChannelEmail, ChannelSlack, ChannelWebhook))
	add(enumOf("DrillStatus", "DRILL_STATUSES", DrillRunning, DrillDone, DrillFailed, DrillSkipped))
	add(enumOf("ExportStatus", "EXPORT_STATUSES", ExportQueued, ExportRunning, ExportDone, ExportFailed, ExportExpired))
	add(enumOf("EmailProvider", "EMAIL_PROVIDERS", EmailPlatform, EmailSMTP))
	add(enumOf("EmailDomainStatus", "EMAIL_DOMAIN_STATUSES", EmailDomainPending, EmailDomainVerified, EmailDomainPaused))
	add(enumOf("SenderProvider", "SENDER_PROVIDERS", SenderResend, SenderSMTP, SenderSES, SenderNone))
	add(enumOf("BucketProvider", "BUCKET_PROVIDERS", BucketPlatform, BucketBYO))
	add(enumOf("UploadStatus", "UPLOAD_STATUSES", UploadAuthorised, UploadClean, UploadInfected, UploadUnscanned, UploadMissing))
	add(enumOf("Visibility", "VISIBILITIES", VisibilityPrivate, VisibilityPublic))
	add(enumOf("MediaStatus", "MEDIA_STATUSES", MediaPending, MediaConverting, MediaReady, MediaFailed, MediaHeld))
	add(enumOf("MediaKind", "MEDIA_KINDS", MediaVideo, MediaAudio))
	add(enumOf("PackageSeverity", "PACKAGE_SEVERITIES", SeverityCritical, SeverityHigh, SeverityMedium, SeverityLow, SeverityUnknown))
	add(enumOf("PackageWhere", "PACKAGE_LOCATIONS", FoundInImage, FoundInSource))
	add(enumOf("ScanStatus", "SCAN_STATUSES", ScanQueued, ScanRunning, ScanDone, ScanFailed, ScanSkipped))
	add(enumOf("ScanTrigger", "SCAN_TRIGGERS", ScanOnDeploy, ScanDaily, ScanOnRequest))
	add(enumOf("LogDestinationKind", "LOG_DESTINATION_KINDS", LogToHTTPS, LogToDatadog, LogToOTLP))
	add(enumOf("LogDestinationStatus", "LOG_DESTINATION_STATUSES", LogSending, LogWaiting, LogFailing, LogStopped))
	add(enumOf("CustomerStatus", "CUSTOMER_STATUSES", CustomerInvited, CustomerActive, CustomerBlocked))
	add(enumOf("DeliveryStatus", "DELIVERY_STATUSES", DeliveryQueued, DeliveryDelivered, DeliveryFailed, DeliveryDead, DeliverySkipped))
	add(enumOf("InboxDomainStatus", "INBOX_DOMAIN_STATUSES", InboxDomainPending, InboxDomainVerified))
	add(enumOf("CdnProviderName", "CDN_PROVIDER_NAMES", CdnBunny, CdnNone))
	add(enumOf("CdnStatus", "CDN_STATUSES", CdnOff, CdnStarting, CdnSwitching, CdnLive, CdnStopping))
	add(enumOf("CdnDirectReason", "CDN_DIRECT_REASONS", CdnThrough, CdnBare, CdnBusiness, CdnPreview))
	add(enumOf("GitHubBranchState", "GITHUB_BRANCH_STATES", BranchInSync, BranchWaiting, BranchRefused))
	add(enumOf("GitHubLinkStatus", "GITHUB_LINK_STATUSES", GitHubLinkActive, GitHubLinkBroken))
	add(enumOf("StepKind", "STEP_KINDS", graph.KindRun, graph.KindApproval, graph.KindSleep, graph.KindWait))
	add(enumOf("MoneyStripeState", "MONEY_STRIPE_STATES", MoneyStripeOK, MoneyStripeUnavailable, MoneyStripeNotSet))
	add(enumOf("MoneyAttentionKind", "MONEY_ATTENTION_KINDS", AttentionPaymentFailed, AttentionDispute, AttentionCardExpiring, AttentionTrialEnding, AttentionDrift))
	add(enumOf("MoneyUpcomingWhy", "MONEY_UPCOMING_WHYS", UpcomingTrialEnds, UpcomingRenews, UpcomingPlanEnds))
	add(enumOf("MoneySubscriptionState", "MONEY_SUBSCRIPTION_STATES", SubTrial, SubPaying, SubPaymentFailed, SubEnding, SubComped))
	add(enumOf("MoneyEventKind", "MONEY_EVENT_KINDS", EventCharge, EventTrialInvoice, EventRefund, EventFailed, EventDispute, EventCredit, EventRebate))
	add(enumOf("SentEmailStatus", "SENT_EMAIL_STATUSES", SentEmailSent, SentEmailFailed, SentEmailBounced, SentEmailComplained))
	add(enumOf("MoneyPayoutStatus", "MONEY_PAYOUT_STATUSES", MoneyPayoutPaid, MoneyPayoutInTransit, MoneyPayoutPending, MoneyPayoutFailed))
	add(enumOf("HistoryRange", "HISTORY_RANGES", HistorySixHours, HistoryDay, HistoryWeek, HistoryFortnight))
	add(enumOf("HistoryState", "HISTORY_STATES", HistoryOK, HistoryDown, HistoryNone))
	add(enumOf("HistoryUnit", "HISTORY_UNITS", UnitPercent, UnitPerMinute))
	add(enumOf("ControlPlaneSlot", "CONTROL_PLANE_SLOTS", SlotBlue, SlotGreen))
	add(enumOf("ResourceState", "RESOURCE_STATES", ResourceCreating, ResourceLive, ResourceReleasing, ResourceReleased, ResourceRetained, ResourceAdopted, ResourceFailed))
	add(enumOf("ResourceRelease", "RESOURCE_RELEASES", ReleaseDelete, ReleaseRetain, ReleaseWithParent))
	add(enumOf("ResourceExport", "RESOURCE_EXPORTS", ExportRepo, ExportDump, ExportFiles, ExportNone))
	add(enumOf("CapacityStatus", "CAPACITY_STATUSES", CapacityOK, CapacityNear, CapacityAddServer))
	add(enumOf("CapacityPeakSource", "CAPACITY_PEAK_SOURCES", PeakHistory, PeakNow))
	add(enumOf("CapacityResource", "CAPACITY_RESOURCES", FirstMemory, FirstDisk))
	add(enumOf("UptimeStatus", "UPTIME_STATUSES", UptimeUp, UptimeDown, UptimeUnknown))
	return m
}()

// wire is every type written out, in order: the generics, then the structs by name. Body and
// Detail are contract/errors' error body and its inner object, named ErrorBody and ErrorDetail as
// the dashboard reads them.
var wire = []any{
	generic{name: "Page", param: "T", sample: Page[typeParam]{}},
	generic{name: "List", param: "T", sample: List[typeParam]{}},
	named{"ErrorDetail", werrors.Detail{}}, named{"ErrorBody", werrors.Body{}},
	Org{}, Timeline{}, Support{}, Comp{}, User{}, Member{}, Group{},
	App{}, AppProblem{}, AppStart{}, AppMemory{}, AppPromoted{}, BackupPoint{}, BackupPoints{}, UptimeDay{},
	UptimeIncident{}, Uptime{}, StatusPage{}, StatusPageAnswer{}, StatusPageRequest{}, Environment{}, Build{}, Phase{}, Deploy{}, DeployEvent{}, Warning{},
	Secret{}, SecretVersion{}, SecretRead{}, Grant{}, Access{}, Domain{}, Token{}, Session{},
	Whoami{}, WhoamiIdentity{}, WhoamiToken{}, AuditEvent{}, Node{}, NodeCopy{}, NodeFailover{}, Manifest{}, Validation{},
	DeviceCode{}, DeviceToken{}, DeviceInfo{}, RequestedFrom{}, DeviceCodeRequest{}, GitPassword{},
	DeviceSuggestion{}, Login{}, LoginBusiness{}, LoginBusinessRequest{},
	OrgHome{}, NeedsYou{},
	CreateOrgRequest{}, PatchOrgRequest{}, ConfirmOrgRequest{}, InviteRequest{}, PatchMemberRequest{},
	GroupRequest{}, GroupMembersRequest{}, CreateAppRequest{}, PatchAppRequest{}, ValidateRequest{},
	AccessRequest{}, CreateDeployRequest{}, DeclareSecretRequest{}, SecretValueRequest{},
	RollbackSecretRequest{}, SecretPreviewsRequest{}, ShareSecretRequest{}, SharedSecret{}, DomainRequest{}, DeployKeyResponse{}, DeviceApproveRequest{},
	RegisterNodeRequest{}, RegisterNodeResponse{},
	// onpremise.go
	Licence{}, InstallLicenceRequest{}, SupportDoor{}, OpenSupportDoorRequest{}, SupportDoorSeen{},
	SupportCallRequest{}, SupportCallAnswer{},
	// operator.go
	Hold{}, HoldDecision{}, WebRisk{}, WebRiskRequest{}, AbuseFlag{}, Feedback{}, FeedbackPage{},
	OwnFeedback{}, OwnFeedbackPage{}, FeedbackReceipt{}, CanaryRun{}, HarnessFailure{}, Revenue{},
	PlanRevenue{}, Signup{}, SignupOrg{}, BrokenGlass{}, DrainResult{}, DrainMove{},
	// billing.go
	Plan{}, Invoice{}, NextInvoice{}, Card{}, Trial{}, FreeApp{}, Billing{}, BillingComp{}, BillingPromoted{}, OrgRef{},
	PlanCheckout{}, TrialStart{}, PayPage{}, PostalAddress{}, CustomerDetails{}, CustomerTaxID{},
	PayDetails{}, PayStep{}, ClientBusiness{}, Handover{}, ClientTrial{}, Clients{}, ClientAdded{},
	HandoverMade{}, ClientOverview{}, ClientUsage{}, RebateSettlement{}, Payouts{}, Rebates{},
	StripeAccount{}, StripeKeyRequest{}, BillingDrift{},
	// numbers.go
	Numbers{}, NumberMetric{}, NumberPoint{}, NumberLimits{}, NumberTally{}, NumberBotRule{},
	NumberBotRules{}, BotMarkRequest{}, NumberSearch{}, SearchProblem{}, NumberSearchTally{}, DailyNumbersMail{},
	// workflows.go, with contract/graph's step graph
	named{"Graph", graph.Graph{}}, named{"GraphStep", graph.Step{}},
	Run{}, RunStep{}, Parked{}, RunsHold{}, RunList{}, WorkflowFunction{}, Trigger{}, Drift{},
	FunctionGraph{}, Approval{},
	// org.go
	DeletedApp{}, Branch{}, StartPreviewRequest{}, StartedPreview{}, OrgDomain{}, OrgDomainApp{},
	OrgDomainRequest{}, MintedToken{}, AgentIdentity{}, AgentIdentityMinted{}, Demo{}, DemoInfo{},
	DemoApp{}, OrgSettings{}, SSOSettings{}, DNSRecord{}, OrgSettingsRequest{}, SSORequest{},
	Preferences{}, Trust{}, TrustBackups{}, TrustAccess{}, TrustStatus{}, Drill{}, Rebuild{}, Export{},
	AccountExport{}, ExportProfile{}, ExportMembership{}, ExportSession{}, ExportPasskey{},
	ExportToken{}, ExportAuditEvent{}, ExportFeedback{}, LastOwnerOrg{},
	// data.go
	EmailDomain{}, EmailAttachment{}, EmailStatus{}, EmailProviderRequest{}, EmailProviderSet{}, EmailSender{},
	EmailAllowance{}, EmailAllowancePeriod{}, StorageInfo{}, StorageProviderRequest{},
	StorageProviderSet{}, AppStorage{}, Upload{}, UploadMedia{}, UploadImage{}, MediaLink{},
	MediaLinks{}, MediaLinkKey{}, PackageFinding{},
	PackageCounts{}, PackageScan{}, Packages{}, LogDestination{}, LogDestinationList{},
	LogDestinationRequest{}, LogLine{}, LogPage{}, PlatformLogPage{}, ErrorGroup{}, ErrorSample{}, ErrorFrame{},
	TraceSummary{}, TraceUsage{}, TraceList{}, TraceSpan{}, TraceEvent{}, TraceDetail{}, Customer{},
	CustomerList{}, WebhookSource{}, WebhookEvent{}, DeliveryError{},
	Inbox{}, InboxDomain{}, InboxDomainRequest{}, InboxMessage{},
	// integrations.go
	CdnProvider{}, CdnHostname{}, AppCdn{}, GitHubRepo{}, GitHubInstallation{}, GitHubOverview{},
	GitHubBranch{}, GitHubLink{}, GitHubAppRecord{}, GitHubAppInfo{}, GitHubManifestStart{},
	// money.go
	Money{}, MoneyAttention{}, MoneyUpcoming{}, MoneySubscription{}, MoneyEvent{}, MoneyPayout{},
	MoneyMonth{},
	// history.go
	History{}, HistoryStrip{}, HistorySegment{}, HistoryChart{}, HistorySeries{}, HistoryAlert{},
	NodeHistory{},
	// controlplane.go
	ControlPlane{}, ControlPlaneRecord{}, SlotHealth{}, SlotSeen{},
	// resources.go
	Resource{}, OrgResources{}, ConfirmedResource{}, WaitingResource{}, WaitingResources{},
	// emails.go
	SentEmail{}, SentEmailOrg{}, SentEmailPage{},
	// capacity.go
	Capacity{}, CapacityMemory{}, CapacityDisk{}, CapacityNode{}, CapacityRow{}, CapacityPrices{},
	CapacityRevenue{},
}

// named writes a struct from outside this package under another name.
type named struct {
	name   string
	sample any
}

// render is pure: the TypeScript for types and the enums they use, or why it cannot be written.
func render(types []any, enums map[reflect.Type]enumSet) (string, error) {
	r := renderer{enums: enums, names: map[reflect.Type]string{}}
	for _, t := range types {
		switch v := t.(type) {
		case generic:
			r.names[reflect.TypeOf(v.sample)] = v.name
		case named:
			r.names[reflect.TypeOf(v.sample)] = v.name
		default:
			r.names[reflect.TypeOf(v)] = reflect.TypeOf(v).Name()
		}
	}
	var b strings.Builder
	b.WriteString(header)
	used := r.enumsUsed(types)
	for _, e := range used {
		quoted := make([]string, len(e.values))
		for i, v := range e.values {
			quoted[i] = fmt.Sprintf("%q", v)
		}
		fmt.Fprintf(&b, "\nexport const %s = [%s] as const;\nexport type %s = (typeof %s)[number];\n",
			e.list, strings.Join(quoted, ", "), e.name, e.list)
	}
	for _, t := range types {
		var name, param string
		var rt reflect.Type
		switch v := t.(type) {
		case generic:
			name, param, rt = v.name+"<"+v.param+">", v.param, reflect.TypeOf(v.sample)
		case named:
			name, rt = v.name, reflect.TypeOf(v.sample)
		default:
			rt = reflect.TypeOf(v)
			name = rt.Name()
		}
		body, err := r.object(rt, param)
		if err != nil {
			return "", fmt.Errorf("%s: %w", name, err)
		}
		fmt.Fprintf(&b, "\nexport type %s = %s;\n", name, body)
	}
	return b.String(), nil
}

const header = `// Generated by contract/apitypes from the Go wire types; do not edit. Change the Go types,
// then run: (cd contract && go test ./apitypes -update)
// Times are RFC 3339 strings; a field marked ? is absent when Go omits it.
`

type renderer struct {
	enums map[reflect.Type]enumSet
	names map[reflect.Type]string
}

// enumsUsed is every enum the types reach, in the order first met.
func (r renderer) enumsUsed(types []any) []enumSet {
	var out []enumSet
	seen := map[reflect.Type]bool{}
	var walk func(t reflect.Type)
	walk = func(t reflect.Type) {
		if seen[t] {
			return
		}
		seen[t] = true
		if e, ok := r.enums[t]; ok {
			out = append(out, e)
			return
		}
		switch t.Kind() {
		case reflect.Pointer, reflect.Slice, reflect.Array, reflect.Map:
			walk(t.Elem())
		case reflect.Struct:
			for i := range t.NumField() {
				walk(t.Field(i).Type)
			}
		default:
		}
	}
	for _, t := range types {
		switch v := t.(type) {
		case generic:
			walk(reflect.TypeOf(v.sample))
		case named:
			walk(reflect.TypeOf(v.sample))
		default:
			walk(reflect.TypeOf(v))
		}
	}
	return out
}

var (
	timeType  = reflect.TypeFor[time.Time]()
	paramType = reflect.TypeFor[typeParam]()
	rawType   = reflect.TypeFor[json.RawMessage]()
)

// object is a struct as a TypeScript object type, its fields in Go's order.
func (r renderer) object(t reflect.Type, param string) (string, error) {
	fields, err := r.fields(t, param)
	if err != nil {
		return "", err
	}
	if len(fields) == 0 {
		return "Record<string, never>", nil
	}
	return "{\n" + strings.Join(fields, "\n") + "\n}", nil
}

func (r renderer) fields(t reflect.Type, param string) ([]string, error) {
	var out []string
	for i := range t.NumField() {
		f := t.Field(i)
		if !f.IsExported() {
			continue
		}
		name, opts, _ := strings.Cut(f.Tag.Get("json"), ",")
		if name == "-" && opts == "" {
			continue
		}
		if f.Anonymous && name == "" && f.Type.Kind() == reflect.Struct {
			inner, err := r.fields(f.Type, param)
			if err != nil {
				return nil, err
			}
			out = append(out, inner...)
			continue
		}
		if name == "" {
			name = f.Name
		}
		flags := strings.Split(opts, ",")
		omit := slices.Contains(flags, "omitempty") || slices.Contains(flags, "omitzero")
		ft := f.Type
		nullable := false
		if ft.Kind() == reflect.Pointer {
			ft, nullable = ft.Elem(), !omit
		}
		ts, err := r.expr(ft, param)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", f.Name, err)
		}
		if nullable {
			ts += " | null"
		}
		mark := ""
		if omit {
			mark = "?"
		}
		out = append(out, fmt.Sprintf("  %s%s: %s;", name, mark, ts))
	}
	return out, nil
}

// expr is a field's type as a TypeScript type expression.
func (r renderer) expr(t reflect.Type, param string) (string, error) {
	if t == paramType && param != "" {
		return param, nil
	}
	if t == timeType {
		return "string", nil
	}
	if t == rawType {
		return "unknown", nil
	}
	if e, ok := r.enums[t]; ok {
		return e.name, nil
	}
	if n, ok := r.names[t]; ok {
		return n, nil
	}
	switch t.Kind() {
	case reflect.String:
		if t.PkgPath() != "" {
			return "", fmt.Errorf("%s is a named string with no values listed in enums", t)
		}
		return "string", nil
	case reflect.Bool:
		return "boolean", nil
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64,
		reflect.Float32, reflect.Float64:
		return "number", nil
	case reflect.Interface:
		return "unknown", nil
	case reflect.Pointer:
		inner, err := r.expr(t.Elem(), param)
		return inner + " | null", err
	case reflect.Array:
		inner, err := r.expr(t.Elem(), param)
		return "[" + strings.Join(slices.Repeat([]string{inner}, t.Len()), ", ") + "]", err
	case reflect.Slice:
		inner, err := r.expr(t.Elem(), param)
		if strings.ContainsAny(inner, " |{") {
			return "Array<" + inner + ">", err
		}
		return inner + "[]", err
	case reflect.Map:
		if t.Key().Kind() != reflect.String {
			return "", fmt.Errorf("%s: a JSON object's keys are strings", t)
		}
		inner, err := r.expr(t.Elem(), param)
		return "Record<string, " + inner + ">", err
	case reflect.Struct:
		if t.Name() != "" {
			return "", fmt.Errorf("%s is not among the types written out", t)
		}
		fields, err := r.fields(t, param)
		if err != nil {
			return "", err
		}
		return "{ " + strings.TrimSpace(strings.Join(trimEach(fields), " ")) + " }", nil
	default:
		return "", fmt.Errorf("%s has no JSON shape", t)
	}
}

func trimEach(lines []string) []string {
	out := make([]string, len(lines))
	for i, l := range lines {
		out[i] = strings.TrimSpace(l)
	}
	return out
}
