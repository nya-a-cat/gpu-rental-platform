import { describe, expect, it } from "vitest";

import { formatResourceRecord } from "../format";

describe("formatResourceRecord", () => {
  it("keeps complete seeded and newly created demo GPU records", () => {
    expect(formatResourceRecord("gpu-01")).toBe("GPU-01");
    expect(formatResourceRecord("demo-gpu-101")).toBe("GPU-101");
  });

  it("keeps compact suffixes for real backend object identifiers", () => {
    expect(formatResourceRecord("65f123456789abcdefabcdef")).toBe("ABCDEF");
  });
});
