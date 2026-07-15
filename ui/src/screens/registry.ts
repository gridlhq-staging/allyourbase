import { createElement, type ReactNode } from "react";
import {
  Activity,
  AlertTriangle,
  Anchor,
  Archive,
  ArrowDownToLine,
  BarChart3,
  Bell,
  BellRing,
  Box,
  Building2,
  Cable,
  CalendarClock,
  Code,
  Columns3,
  Compass,
  Database,
  FileText,
  Fingerprint,
  Gauge,
  GitBranch,
  Globe,
  HardDrive,
  KeyRound,
  Layers,
  LifeBuoy,
  LineChart,
  Link,
  ListTodo,
  Lock,
  Mail,
  MessageCircle,
  MessageSquare,
  Puzzle,
  Search,
  Server,
  Settings,
  Shield,
  ShieldAlert,
  ShieldCheck,
  ShieldPlus,
  Sparkles,
  ScrollText,
  Users as UsersIcon,
  Webhook,
  Zap,
  type LucideIcon,
} from "lucide-react";
import type { SchemaCache } from "../types";
import { AccountLinking } from "../components/AccountLinking";
import { AdminLogs } from "../components/AdminLogs";
import { AIAssistant } from "../components/AIAssistant";
import { Analytics } from "../components/Analytics";
import { ApiExplorer } from "../components/ApiExplorer";
import { ApiKeys } from "../components/ApiKeys";
import { Apps } from "../components/Apps";
import { AuditLogs } from "../components/AuditLogs";
import { AuthHooks } from "../components/AuthHooks";
import { AuthSettings } from "../components/AuthSettings";
import { Backups } from "../components/Backups";
import { Branches } from "../components/Branches";
import { CustomDomains } from "../components/CustomDomains";
import { EdgeFunctions } from "../components/EdgeFunctions";
import { EmailTemplates } from "../components/EmailTemplates";
import { Extensions } from "../components/Extensions";
import { FDWManagement } from "../components/FDWManagement";
import { FunctionBrowser } from "../components/FunctionBrowser";
import { Incidents } from "../components/Incidents";
import { Jobs } from "../components/Jobs";
import { LogDrains } from "../components/LogDrains";
import { MatviewsAdmin } from "../components/MatviewsAdmin";
import { MFAEnrollment } from "../components/MFAEnrollment";
import { Notifications } from "../components/Notifications";
import { OAuthClients } from "../components/OAuthClients";
import { Organizations } from "../components/Organizations";
import { PerformanceAdvisor } from "../components/PerformanceAdvisor";
import { PushNotifications } from "../components/PushNotifications";
import { RealtimeInspector } from "../components/RealtimeInspector";
import { Replicas } from "../components/Replicas";
import { RlsPolicies } from "../components/RlsPolicies";
import { SAMLConfig } from "../components/SAMLConfig";
import { Schedules } from "../components/Schedules";
import { SchemaDesigner } from "../components/SchemaDesigner";
import { Search as SearchScreen } from "../components/Search";
import { Secrets } from "../components/Secrets";
import { SecurityAdvisor } from "../components/SecurityAdvisor";
import { Sites } from "../components/Sites";
import { SMSHealth } from "../components/SMSHealth";
import { SMSMessages } from "../components/SMSMessages";
import { SqlEditor } from "../components/SqlEditor";
import { StatsOverview } from "../components/StatsOverview";
import { StorageBrowser } from "../components/StorageBrowser";
import { SupportTickets } from "../components/SupportTickets";
import { Tenants } from "../components/Tenants";
import { UsageMetering } from "../components/UsageMetering";
import { Users } from "../components/Users";
import { VectorIndexes } from "../components/VectorIndexes";
import { Webhooks } from "../components/Webhooks";
import type { AdminCapabilityName } from "../api_capabilities";

export const ADMIN_VIEWS = [
  "webhooks",
  "storage",
  "sites",
  "users",
  "functions",
  "edge-functions",
  "apps",
  "api-keys",
  "oauth-clients",
  "api-explorer",
  "rls",
  "sql-editor",
  "schema-designer",
  "sms-health",
  "sms-messages",
  "email-templates",
  "push",
  "jobs",
  "schedules",
  "matviews",
  "auth-settings",
  "mfa-management",
  "account-linking",
  "branches",
  "realtime-inspector",
  "security-advisor",
  "performance-advisor",
  "backups",
  "analytics",
  "usage",
  "replicas",
  "ai-assistant",
  "audit-logs",
  "admin-logs",
  "secrets",
  "saml",
  "custom-domains",
  "extensions",
  "search",
  "vector-indexes",
  "log-drains",
  "stats",
  "auth-hooks",
  "notifications",
  "fdw",
  "incidents",
  "support-tickets",
  "tenants",
  "organizations",
] as const;

type AdminScreenId = (typeof ADMIN_VIEWS)[number];

export interface ScreenProps {
  schema: SchemaCache;
  onRefresh: () => void | Promise<void>;
}

export interface AdminScreen {
  readonly id: AdminScreenId;
  readonly label: string;
  readonly icon: LucideIcon;
  readonly testId?: string;
  readonly requires?: AdminCapabilityName;
  readonly render: (props: ScreenProps) => ReactNode;
}

export interface ScreenSection {
  readonly title: "Database" | "Services" | "Messaging" | "Admin" | "AI" | "Auth";
  readonly screens: readonly AdminScreen[];
}

export interface ScreenRegistry {
  readonly sections: readonly ScreenSection[];
}

export const SCREEN_REGISTRY: ScreenRegistry = {
  sections: [
    {
      title: "Database",
      screens: [
        { id: "sql-editor", label: "SQL Editor", icon: Code, render: ({ onRefresh }) => createElement(SqlEditor, { onSchemaChange: onRefresh }) },
        { id: "functions", label: "Functions", icon: Zap, render: ({ schema }) => createElement(FunctionBrowser, { functions: schema.functions || {} }) },
        { id: "rls", label: "RLS Policies", icon: Shield, render: ({ schema }) => createElement(RlsPolicies, { schema }) },
        { id: "search", label: "Search", icon: Search, render: ({ schema }) => createElement(SearchScreen, { schema }) },
        { id: "matviews", label: "Matviews", icon: Layers, render: ({ schema }) => createElement(MatviewsAdmin, { schema }) },
        { id: "schema-designer", label: "Schema Designer", icon: Columns3, testId: "nav-schema-designer", render: ({ schema }) => createElement(SchemaDesigner, { schema }) },
        { id: "fdw", label: "FDW", icon: Cable, render: () => createElement(FDWManagement) },
      ],
    },
    {
      title: "Services",
      screens: [
        { id: "storage", label: "Storage", icon: HardDrive, render: () => createElement(StorageBrowser) },
        { id: "sites", label: "Sites", icon: Globe, render: () => createElement(Sites) },
        { id: "edge-functions", label: "Edge Functions", icon: Zap, render: () => createElement(EdgeFunctions) },
        { id: "webhooks", label: "Webhooks", icon: Webhook, render: () => createElement(Webhooks) },
      ],
    },
    {
      title: "Messaging",
      screens: [
        { id: "sms-health", label: "SMS Health", icon: MessageCircle, render: () => createElement(SMSHealth) },
        { id: "sms-messages", label: "SMS Messages", icon: MessageSquare, render: () => createElement(SMSMessages) },
        { id: "email-templates", label: "Email Templates", icon: Mail, render: () => createElement(EmailTemplates) },
        { id: "push", label: "Push Notifications", icon: Bell, render: () => createElement(PushNotifications) },
      ],
    },
    {
      title: "Admin",
      screens: [
        { id: "users", label: "Users", icon: UsersIcon, render: () => createElement(Users) },
        { id: "apps", label: "Apps", icon: Box, render: () => createElement(Apps) },
        { id: "api-keys", label: "API Keys", icon: KeyRound, render: () => createElement(ApiKeys) },
        { id: "oauth-clients", label: "OAuth Clients", icon: Fingerprint, render: () => createElement(OAuthClients) },
        { id: "api-explorer", label: "API Explorer", icon: Compass, render: ({ schema }) => createElement(ApiExplorer, { schema }) },
        { id: "jobs", label: "Jobs", icon: ListTodo, render: () => createElement(Jobs) },
        { id: "schedules", label: "Schedules", icon: CalendarClock, render: () => createElement(Schedules) },
        { id: "realtime-inspector", label: "Realtime Inspector", icon: Activity, render: () => createElement(RealtimeInspector) },
        { id: "security-advisor", label: "Security Advisor", icon: ShieldAlert, render: () => createElement(SecurityAdvisor) },
        { id: "performance-advisor", label: "Performance Advisor", icon: Gauge, render: () => createElement(PerformanceAdvisor) },
        { id: "backups", label: "Backups", icon: Archive, render: () => createElement(Backups) },
        { id: "analytics", label: "Analytics", icon: BarChart3, render: () => createElement(Analytics) },
        { id: "usage", label: "Usage", icon: LineChart, render: () => createElement(UsageMetering) },
        { id: "replicas", label: "Replicas", icon: Server, render: () => createElement(Replicas) },
        { id: "branches", label: "Branches", icon: GitBranch, render: () => createElement(Branches) },
        { id: "audit-logs", label: "Audit Logs", icon: ScrollText, render: () => createElement(AuditLogs) },
        { id: "admin-logs", label: "Admin Logs", icon: FileText, render: () => createElement(AdminLogs) },
        { id: "secrets", label: "Secrets", icon: Lock, render: () => createElement(Secrets) },
        { id: "custom-domains", label: "Custom Domains", icon: Globe, render: () => createElement(CustomDomains) },
        { id: "extensions", label: "Extensions", icon: Puzzle, render: () => createElement(Extensions) },
        { id: "vector-indexes", label: "Vector Indexes", icon: Database, render: () => createElement(VectorIndexes) },
        { id: "log-drains", label: "Log Drains", icon: ArrowDownToLine, render: () => createElement(LogDrains) },
        { id: "stats", label: "Stats", icon: LineChart, render: () => createElement(StatsOverview) },
        { id: "notifications", label: "Notifications", icon: BellRing, render: () => createElement(Notifications) },
        { id: "incidents", label: "Incidents", icon: AlertTriangle, render: () => createElement(Incidents) },
        { id: "support-tickets", label: "Support Tickets", icon: LifeBuoy, render: () => createElement(SupportTickets) },
        { id: "tenants", label: "Tenants", icon: Building2, render: () => createElement(Tenants) },
        { id: "organizations", label: "Organizations", icon: Building2, render: () => createElement(Organizations) },
      ],
    },
    {
      title: "AI",
      screens: [
        { id: "ai-assistant", label: "AI Assistant", icon: Sparkles, render: () => createElement(AIAssistant) },
      ],
    },
    {
      title: "Auth",
      screens: [
        { id: "auth-settings", label: "Auth Settings", icon: Settings, render: () => createElement(AuthSettings) },
        { id: "mfa-management", label: "MFA Management", icon: ShieldCheck, render: () => createElement(MFAEnrollment) },
        { id: "account-linking", label: "Account Linking", icon: Link, render: () => createElement(AccountLinking, { onLinked: () => {} }) },
        { id: "saml", label: "SAML", icon: ShieldPlus, render: () => createElement(SAMLConfig) },
        { id: "auth-hooks", label: "Auth Hooks", icon: Anchor, render: () => createElement(AuthHooks) },
      ],
    },
  ],
};

export function filterScreenRegistry(
  registry: ScreenRegistry,
  canUse: (capability: AdminCapabilityName) => boolean,
): ScreenRegistry {
  const sections = registry.sections.flatMap((section) => {
    const screens = section.screens.filter((screen) => (
      screen.requires ? canUse(screen.requires) : true
    ));
    return screens.length > 0 ? [{ ...section, screens }] : [];
  });
  return { sections };
}

export function findAdminScreen(
  screenId: string,
  registry: ScreenRegistry = SCREEN_REGISTRY,
): AdminScreen | undefined {
  for (const section of registry.sections) {
    const screen = section.screens.find((candidate) => candidate.id === screenId);
    if (screen) {
      return screen;
    }
  }
  return undefined;
}
