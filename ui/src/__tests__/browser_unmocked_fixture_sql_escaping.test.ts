import { describe, it, expect } from "vitest";
import { escapeLikePattern, sqlLiteral } from "../../browser-tests-unmocked/fixtures";

// These helpers are the shared owner for building SQL string literals and LIKE
// patterns in the unmocked browser fixtures. The rest of the fixture suite only
// ever grep the source text for their names, so this file is the sole place
// their actual behavior is asserted. Expected values are hand-calculated.
describe("shared fixture SQL escaping", () => {
  describe("sqlLiteral", () => {
    it("doubles single quotes so the value survives a quoted SQL literal", () => {
      expect(sqlLiteral("O'Brien")).toBe("O''Brien");
      expect(sqlLiteral("''")).toBe("''''");
    });

    it("leaves LIKE wildcards alone — quoting is not pattern escaping", () => {
      expect(sqlLiteral("100%_x")).toBe("100%_x");
    });
  });

  describe("escapeLikePattern", () => {
    it("passes through a value with nothing to escape", () => {
      expect(escapeLikePattern("fixture-token")).toBe("fixture-token");
    });

    it("neutralizes the LIKE wildcards so they match literally", () => {
      // Callers embed the result in `LIKE '%<pattern>%' ESCAPE '\'`, so a literal
      // % or _ in the input must arrive at Postgres backslash-prefixed.
      expect(escapeLikePattern("50%")).toBe("50\\%");
      expect(escapeLikePattern("a_b")).toBe("a\\_b");
    });

    it("escapes the escape character itself", () => {
      expect(escapeLikePattern("c:\\path")).toBe("c:\\\\path");
    });

    it("quote-escapes so the result is safe to interpolate into the quoted pattern", () => {
      expect(escapeLikePattern("abc'def")).toBe("abc''def");
    });

    it("escapes backslashes before wildcards, so added escapes are not re-escaped", () => {
      // Ordering regression guard. Input chars: 1 0 0 % _ \
      // Correct (backslash first): 1 0 0 \ % \ _ \ \
      // If % were escaped first, the backslash pass would double the escape it
      // had just introduced, and Postgres would match a literal backslash
      // followed by the wildcard rather than a literal '%'.
      expect(escapeLikePattern("100%_\\")).toBe("100\\%\\_\\\\");
    });

    it("handles a value needing every escape at once", () => {
      expect(escapeLikePattern("O'Brien_50%\\")).toBe("O''Brien\\_50\\%\\\\");
    });
  });
});
