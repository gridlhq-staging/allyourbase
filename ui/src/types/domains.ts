/**
 * @module ui/src/types/domains.ts
 */
export interface DomainBinding {
  id: string;
  hostname: string;
  environment: string;
  status: string;
  verificationToken: string;
  verificationRecord?: string;
  certRef?: string;
  certExpiry?: string;
  redirectMode?: string;
  lastError?: string;
  healthStatus: string;
  lastHealthCheck?: string;
  createdAt: string;
  updatedAt: string;
}

export interface DomainBindingListResult {
  items: DomainBinding[];
  page: number;
  perPage: number;
  totalItems: number;
  totalPages: number;
}

export interface CreateDomainRequest {
  hostname: string;
  environment: string;
  redirectMode: string;
}
