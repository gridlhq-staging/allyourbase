import {
  createElement,
  lazy,
  Suspense,
  useMemo,
  type ComponentType,
  type ReactNode,
} from "react";
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
  Braces,
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
  "graphql",
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

type LazyScreenModule<TProps extends object> = {
  readonly default: ComponentType<TProps>;
};

type LazyScreenLoader<TProps extends object> = () => Promise<LazyScreenModule<TProps>>;
type RejectedChunkImporter = (url: string) => Promise<unknown>;
const LAZY_SCREEN_ASSET_BASE_URL = new URL(".", import.meta.url);

interface RetryableLazyScreenProps<TProps extends object> {
  readonly load: LazyScreenLoader<TProps>;
  readonly mapScreenProps: (props: ScreenProps) => TProps;
  readonly screenProps: ScreenProps;
}

interface LazyScreenRenderOptions {
  readonly exportName?: string;
  readonly importRejectedChunk?: RejectedChunkImporter;
}

function LoadingScreenFallback() {
  return createElement(
    "div",
    {
      role: "status",
      "aria-label": "Loading screen",
      className: "flex min-h-48 items-center justify-center text-sm text-gray-500 dark:text-gray-400",
    },
    "Loading...",
  );
}

function RetryableLazyScreen<TProps extends object>({
  load,
  mapScreenProps,
  screenProps,
}: RetryableLazyScreenProps<TProps>) {
  const LazyScreen = useMemo(() => lazy(load), [load]);
  const LazyScreenComponent = LazyScreen as unknown as ComponentType<TProps>;
  return createElement(
    Suspense,
    { fallback: createElement(LoadingScreenFallback) },
    createElement(LazyScreenComponent, mapScreenProps(screenProps)),
  );
}

function failedChunkURL(error: unknown): string | undefined {
  if (!(error instanceof Error)) {
    return undefined;
  }
  const match = error.message.match(/https?:\/\/\S+?\.js(?:\?\S*)?/);
  return sanitizeRetryChunkURL(match?.[0]);
}

function sanitizeRetryChunkURL(url: string | undefined): string | undefined {
  if (!url) {
    return undefined;
  }
  try {
    const parsed = new URL(url, LAZY_SCREEN_ASSET_BASE_URL);
    if (parsed.origin !== LAZY_SCREEN_ASSET_BASE_URL.origin) {
      return undefined;
    }
    if (parsed.protocol !== LAZY_SCREEN_ASSET_BASE_URL.protocol) {
      return undefined;
    }
    if (!parsed.pathname.endsWith(".js")) {
      return undefined;
    }
    if (!parsed.pathname.startsWith(LAZY_SCREEN_ASSET_BASE_URL.pathname)) {
      return undefined;
    }
    return parsed.toString();
  } catch {
    return undefined;
  }
}

function freshChunkURL(url: string): string {
  const separator = url.includes("?") ? "&" : "?";
  return `${url}${separator}ayb_retry=${Date.now()}`;
}

function moduleExports(module: unknown): Record<string, unknown> {
  if (typeof module !== "object" || module === null) {
    throw new Error("Lazy screen chunk did not export a module object");
  }
  return module as Record<string, unknown>;
}

function moduleWithNamedDefault<TProps extends object>(
  module: Record<string, unknown>,
  exportName: string,
): LazyScreenModule<TProps> {
  const component = module[exportName];
  if (typeof component !== "function") {
    throw new Error(`Lazy screen chunk did not export ${exportName}`);
  }
  return { default: component as ComponentType<TProps> };
}

function moduleWithDefault<TProps extends object>(
  module: Record<string, unknown>,
  exportName?: string,
): LazyScreenModule<TProps> {
  if (exportName) {
    return moduleWithNamedDefault<TProps>(module, exportName);
  }

  if (typeof module.default === "function") {
    return { default: module.default as ComponentType<TProps> };
  }

  throw new Error("Lazy screen chunk did not export a default React component");
}

function importRejectedChunk(url: string): Promise<unknown> {
  return import(/* @vite-ignore */ url);
}

export function createLazyScreenRender<TProps extends object>(
  load: LazyScreenLoader<TProps>,
  mapScreenProps: (props: ScreenProps) => TProps = () => ({} as TProps),
  options: LazyScreenRenderOptions = {},
): AdminScreen["render"] {
  let rejectedChunkURL: string | undefined;
  const retryableLoad = async () => {
    if (rejectedChunkURL) {
      const retryImporter = options.importRejectedChunk ?? importRejectedChunk;
      const module = await retryImporter(freshChunkURL(rejectedChunkURL));
      rejectedChunkURL = undefined;
      return moduleWithDefault<TProps>(moduleExports(module), options.exportName);
    }

    try {
      return await load();
    } catch (error) {
      rejectedChunkURL = failedChunkURL(error);
      throw error;
    }
  };

  return (screenProps) =>
    createElement(RetryableLazyScreen<TProps>, {
      load: retryableLoad,
      mapScreenProps,
      screenProps,
    });
}

function createNamedLazyScreenRender<TProps extends object>(
  loadModule: () => Promise<unknown>,
  exportName: string,
  mapScreenProps: (props: ScreenProps) => TProps = () => ({} as TProps),
): AdminScreen["render"] {
  return createLazyScreenRender<TProps>(
    () =>
      loadModule().then((module) =>
        moduleWithNamedDefault<TProps>(moduleExports(module), exportName),
      ),
    mapScreenProps,
    { exportName },
  );
}

export const SCREEN_REGISTRY: ScreenRegistry = {
  sections: [
    {
      title: "Database",
      screens: [
        { id: "sql-editor", label: "SQL Editor", icon: Code, render: createNamedLazyScreenRender(() => import("../components/SqlEditor"), "SqlEditor", ({ onRefresh }) => ({ onSchemaChange: onRefresh })) },
        { id: "graphql", label: "GraphQL", icon: Braces, render: createNamedLazyScreenRender(() => import("../components/GraphqlExplorer"), "GraphqlExplorer") },
        { id: "functions", label: "Functions", icon: Zap, render: createNamedLazyScreenRender(() => import("../components/FunctionBrowser"), "FunctionBrowser", ({ schema }) => ({ functions: schema.functions || {} })) },
        { id: "rls", label: "RLS Policies", icon: Shield, render: createNamedLazyScreenRender(() => import("../components/RlsPolicies"), "RlsPolicies", ({ schema }) => ({ schema })) },
        { id: "search", label: "Search", icon: Search, render: createNamedLazyScreenRender(() => import("../components/Search"), "Search", ({ schema }) => ({ schema })) },
        { id: "matviews", label: "Matviews", icon: Layers, render: createNamedLazyScreenRender(() => import("../components/MatviewsAdmin"), "MatviewsAdmin", ({ schema }) => ({ schema })) },
        { id: "schema-designer", label: "Schema Designer", icon: Columns3, testId: "nav-schema-designer", render: createNamedLazyScreenRender(() => import("../components/SchemaDesigner"), "SchemaDesigner", ({ schema }) => ({ schema })) },
        { id: "fdw", label: "FDW", icon: Cable, render: createNamedLazyScreenRender(() => import("../components/FDWManagement"), "FDWManagement") },
      ],
    },
    {
      title: "Services",
      screens: [
        { id: "storage", label: "Storage", icon: HardDrive, render: createNamedLazyScreenRender(() => import("../components/StorageBrowser"), "StorageBrowser") },
        { id: "sites", label: "Sites", icon: Globe, render: createNamedLazyScreenRender(() => import("../components/Sites"), "Sites") },
        { id: "edge-functions", label: "Edge Functions", icon: Zap, render: createNamedLazyScreenRender(() => import("../components/EdgeFunctions"), "EdgeFunctions") },
        { id: "webhooks", label: "Webhooks", icon: Webhook, render: createNamedLazyScreenRender(() => import("../components/Webhooks"), "Webhooks") },
      ],
    },
    {
      title: "Messaging",
      screens: [
        { id: "sms-health", label: "SMS Health", icon: MessageCircle, render: createNamedLazyScreenRender(() => import("../components/SMSHealth"), "SMSHealth") },
        { id: "sms-messages", label: "SMS Messages", icon: MessageSquare, render: createNamedLazyScreenRender(() => import("../components/SMSMessages"), "SMSMessages") },
        { id: "email-templates", label: "Email Templates", icon: Mail, render: createNamedLazyScreenRender(() => import("../components/EmailTemplates"), "EmailTemplates") },
        { id: "push", label: "Push Notifications", icon: Bell, render: createNamedLazyScreenRender(() => import("../components/PushNotifications"), "PushNotifications") },
      ],
    },
    {
      title: "Admin",
      screens: [
        { id: "users", label: "Users", icon: UsersIcon, render: createNamedLazyScreenRender(() => import("../components/Users"), "Users") },
        { id: "apps", label: "Apps", icon: Box, render: createNamedLazyScreenRender(() => import("../components/Apps"), "Apps") },
        { id: "api-keys", label: "API Keys", icon: KeyRound, render: createNamedLazyScreenRender(() => import("../components/ApiKeys"), "ApiKeys") },
        { id: "oauth-clients", label: "OAuth Clients", icon: Fingerprint, render: createNamedLazyScreenRender(() => import("../components/OAuthClients"), "OAuthClients") },
        { id: "api-explorer", label: "API Explorer", icon: Compass, render: createNamedLazyScreenRender(() => import("../components/ApiExplorer"), "ApiExplorer", ({ schema }) => ({ schema })) },
        { id: "jobs", label: "Jobs", icon: ListTodo, render: createNamedLazyScreenRender(() => import("../components/Jobs"), "Jobs") },
        { id: "schedules", label: "Schedules", icon: CalendarClock, render: createNamedLazyScreenRender(() => import("../components/Schedules"), "Schedules") },
        { id: "realtime-inspector", label: "Realtime Inspector", icon: Activity, render: createNamedLazyScreenRender(() => import("../components/RealtimeInspector"), "RealtimeInspector") },
        { id: "security-advisor", label: "Security Advisor", icon: ShieldAlert, render: createNamedLazyScreenRender(() => import("../components/SecurityAdvisor"), "SecurityAdvisor") },
        { id: "performance-advisor", label: "Performance Advisor", icon: Gauge, render: createNamedLazyScreenRender(() => import("../components/PerformanceAdvisor"), "PerformanceAdvisor") },
        { id: "backups", label: "Backups", icon: Archive, render: createNamedLazyScreenRender(() => import("../components/Backups"), "Backups") },
        { id: "analytics", label: "Analytics", icon: BarChart3, render: createNamedLazyScreenRender(() => import("../components/Analytics"), "Analytics") },
        { id: "usage", label: "Usage", icon: LineChart, render: createNamedLazyScreenRender(() => import("../components/UsageMetering"), "UsageMetering") },
        { id: "replicas", label: "Replicas", icon: Server, render: createNamedLazyScreenRender(() => import("../components/Replicas"), "Replicas") },
        { id: "branches", label: "Branches", icon: GitBranch, render: createNamedLazyScreenRender(() => import("../components/Branches"), "Branches") },
        { id: "audit-logs", label: "Audit Logs", icon: ScrollText, render: createNamedLazyScreenRender(() => import("../components/AuditLogs"), "AuditLogs") },
        { id: "admin-logs", label: "Admin Logs", icon: FileText, render: createNamedLazyScreenRender(() => import("../components/AdminLogs"), "AdminLogs") },
        { id: "secrets", label: "Secrets", icon: Lock, render: createNamedLazyScreenRender(() => import("../components/Secrets"), "Secrets") },
        { id: "custom-domains", label: "Custom Domains", icon: Globe, render: createNamedLazyScreenRender(() => import("../components/CustomDomains"), "CustomDomains") },
        { id: "extensions", label: "Extensions", icon: Puzzle, render: createNamedLazyScreenRender(() => import("../components/Extensions"), "Extensions") },
        { id: "vector-indexes", label: "Vector Indexes", icon: Database, render: createNamedLazyScreenRender(() => import("../components/VectorIndexes"), "VectorIndexes") },
        { id: "log-drains", label: "Log Drains", icon: ArrowDownToLine, render: createNamedLazyScreenRender(() => import("../components/LogDrains"), "LogDrains") },
        { id: "stats", label: "Stats", icon: LineChart, render: createNamedLazyScreenRender(() => import("../components/StatsOverview"), "StatsOverview") },
        { id: "notifications", label: "Notifications", icon: BellRing, render: createNamedLazyScreenRender(() => import("../components/Notifications"), "Notifications") },
        { id: "incidents", label: "Incidents", icon: AlertTriangle, render: createNamedLazyScreenRender(() => import("../components/Incidents"), "Incidents") },
        { id: "support-tickets", label: "Support Tickets", icon: LifeBuoy, render: createNamedLazyScreenRender(() => import("../components/SupportTickets"), "SupportTickets") },
        { id: "tenants", label: "Tenants", icon: Building2, render: createNamedLazyScreenRender(() => import("../components/Tenants"), "Tenants") },
        { id: "organizations", label: "Organizations", icon: Building2, render: createNamedLazyScreenRender(() => import("../components/Organizations"), "Organizations") },
      ],
    },
    {
      title: "AI",
      screens: [
        { id: "ai-assistant", label: "AI Assistant", icon: Sparkles, render: createNamedLazyScreenRender(() => import("../components/AIAssistant"), "AIAssistant") },
      ],
    },
    {
      title: "Auth",
      screens: [
        { id: "auth-settings", label: "Auth Settings", icon: Settings, render: createNamedLazyScreenRender(() => import("../components/AuthSettings"), "AuthSettings") },
        { id: "mfa-management", label: "MFA Management", icon: ShieldCheck, render: createNamedLazyScreenRender(() => import("../components/MFAEnrollment"), "MFAEnrollment") },
        { id: "account-linking", label: "Account Linking", icon: Link, render: createNamedLazyScreenRender(() => import("../components/AccountLinking"), "AccountLinking", () => ({ onLinked: () => {} })) },
        { id: "saml", label: "SAML", icon: ShieldPlus, render: createNamedLazyScreenRender(() => import("../components/SAMLConfig"), "SAMLConfig") },
        { id: "auth-hooks", label: "Auth Hooks", icon: Anchor, render: createNamedLazyScreenRender(() => import("../components/AuthHooks"), "AuthHooks") },
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
