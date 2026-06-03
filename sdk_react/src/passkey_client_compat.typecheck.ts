import type { AYBClientLike } from "./types";

const legacyClientWithoutPasskey: AYBClientLike = {
  token: null,
  refreshToken: null,
  onAuthStateChange: () => () => {},
  auth: {
    login: async () => ({}),
    register: async () => ({}),
    signInAnonymously: async () => ({}),
    requestMagicLink: async () => ({}),
    confirmMagicLink: async () => ({}),
    linkEmail: async () => ({}),
    signInWithOAuth: async () => ({}),
    logout: async () => {},
    refresh: async () => ({}),
    me: async () => ({ id: "legacy-user" }),
  },
  records: {
    list: async () => ({ items: [], page: 1, perPage: 20, totalItems: 0, totalPages: 0 }),
  },
  realtime: {
    subscribe: () => () => {},
  },
};

void legacyClientWithoutPasskey;
