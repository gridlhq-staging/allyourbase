import { randomUUID } from "crypto";
import type { APIRequestContext } from "@playwright/test";
import { assertSafeSQLIdentifier, execSQL } from "./core";

const POSTGRES_IDENTIFIER_LIMIT = 63;

export interface IsolatedAdminRelation {
  removeEmptyClone: () => Promise<void>;
  restore: () => Promise<void>;
}

function createBackupRelationName(relationName: string): string {
  const suffix = `_e2e_${randomUUID().replaceAll("-", "").slice(0, 12)}`;
  return `${relationName.slice(0, POSTGRES_IDENTIFIER_LIMIT - suffix.length)}${suffix}`;
}

class AdminRelationIsolation implements IsolatedAdminRelation {
  private state: "empty_clone" | "missing" | "restored" = "empty_clone";

  constructor(
    private readonly request: APIRequestContext,
    private readonly token: string,
    private readonly relationName: string,
    private readonly backupRelationName: string,
  ) {}

  async removeEmptyClone(): Promise<void> {
    if (this.state !== "empty_clone") {
      throw new Error(`Cannot remove ${this.relationName} while relation state is ${this.state}`);
    }
    await execSQL(this.request, this.token, `DROP TABLE ${this.relationName}`);
    this.state = "missing";
  }

  async restore(): Promise<void> {
    if (this.state === "restored") {
      return;
    }
    if (this.state === "empty_clone") {
      await execSQL(this.request, this.token, `DROP TABLE ${this.relationName}`);
    }
    await execSQL(
      this.request,
      this.token,
      `ALTER TABLE ${this.backupRelationName} RENAME TO ${this.relationName}`,
    );
    this.state = "restored";
  }
}

export async function replaceAdminRelationWithEmptyClone(
  request: APIRequestContext,
  token: string,
  relationName: string,
): Promise<IsolatedAdminRelation> {
  const safeRelationName = assertSafeSQLIdentifier(relationName, "admin relation");
  const backupRelationName = assertSafeSQLIdentifier(
    createBackupRelationName(safeRelationName),
    "admin relation backup",
  );

  await execSQL(
    request,
    token,
    `ALTER TABLE ${safeRelationName} RENAME TO ${backupRelationName}`,
  );
  try {
    await execSQL(
      request,
      token,
      `CREATE TABLE ${safeRelationName} (LIKE ${backupRelationName} INCLUDING ALL)`,
    );
  } catch (creationError) {
    try {
      await execSQL(
        request,
        token,
        `ALTER TABLE ${backupRelationName} RENAME TO ${safeRelationName}`,
      );
    } catch (restorationError) {
      throw new AggregateError(
        [creationError, restorationError],
        `Failed to create isolated ${safeRelationName} and restore its original relation`,
      );
    }
    throw creationError;
  }

  return new AdminRelationIsolation(
    request,
    token,
    safeRelationName,
    backupRelationName,
  );
}
