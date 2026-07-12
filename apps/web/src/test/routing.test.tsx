import { render, screen } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { describe, expect, it } from "vitest";

import { AppRoutes } from "../App";
import { AppProviders } from "../app-context";
import { DemoGateway, type StorageLike } from "../data/demo-gateway";

class MemoryStorage implements StorageLike {
  private readonly data = new Map<string, string>();
  getItem(key: string): string | null {
    return this.data.get(key) ?? null;
  }
  removeItem(key: string): void {
    this.data.delete(key);
  }
  setItem(key: string, value: string): void {
    this.data.set(key, value);
  }
}

describe("role routes", () => {
  it("redirects a signed-in operator away from the admin console", async () => {
    const storage = new MemoryStorage();
    const gateway = new DemoGateway(storage);
    await gateway.resetDemo();
    const key = "gpu-rental-demo-state-v1";
    const state = JSON.parse(storage.getItem(key)!) as {
      currentUserId: string | null;
    };
    state.currentUserId = "demo-user";
    storage.setItem(key, JSON.stringify(state));

    render(
      <MemoryRouter initialEntries={["/admin"]}>
        <AppProviders gateway={gateway}>
          <AppRoutes />
        </AppProviders>
      </MemoryRouter>,
    );

    expect(await screen.findByText("访问被拒绝")).toBeInTheDocument();
  });
});
