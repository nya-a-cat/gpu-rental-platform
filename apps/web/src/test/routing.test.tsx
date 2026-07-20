import { GpuAvailability } from "@gpu-rental/contracts";
import {
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { beforeEach, describe, expect, it } from "vitest";

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
  beforeEach(() => {
    Object.defineProperty(window, "localStorage", {
      configurable: true,
      value: new MemoryStorage(),
    });
  });

  it("redirects a signed-in operator away from the admin console", async () => {
    const storage = new MemoryStorage();
    const gateway = new DemoGateway(storage);
    await gateway.resetDemo();
    const key = "gpu-rental-demo-state-v2";
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

  it("updates inventory filters through the mechanical console", async () => {
    const gateway = new DemoGateway(new MemoryStorage());
    await gateway.resetDemo();

    render(
      <MemoryRouter initialEntries={["/"]}>
        <AppProviders gateway={gateway}>
          <AppRoutes />
        </AppProviders>
      </MemoryRouter>,
    );

    const stateControl = await screen.findByRole("button", {
      name: "资源状态: 全部",
    });
    const quickRack = screen.getByRole("region", {
      name: "实时资源快速入口",
    });
    expect(
      await within(quickRack).findAllByRole("link", { name: /打开资源/ }),
    ).toHaveLength(3);
    expect(screen.getByRole("meter", { name: "控制偏移" })).toHaveAttribute(
      "aria-valuetext",
      "0/6",
    );
    fireEvent.click(stateControl);

    await waitFor(() => {
      expect(
        (screen.getByLabelText("资源状态") as HTMLSelectElement).value,
      ).toBe(GpuAvailability.Available);
    });
    expect(
      screen.getByRole("button", { name: "资源状态: 可预订" }),
    ).toHaveAttribute("data-position", "1");
    expect(await within(quickRack).findByText("5 台匹配")).toBeInTheDocument();
    expect(screen.getByRole("meter", { name: "控制偏移" })).toHaveAttribute(
      "aria-valuetext",
      "1/6",
    );

    fireEvent.keyDown(
      screen.getByRole("button", { name: "资源状态: 可预订" }),
      { key: "ArrowLeft" },
    );
    await waitFor(() => {
      expect(
        (screen.getByLabelText("资源状态") as HTMLSelectElement).value,
      ).toBe("");
    });

    const priceControl = await screen.findByRole("button", {
      name: "价格上限: ¥32.90",
    });
    const powerControl = screen.getByRole("button", {
      name: "控制总线接通",
    });
    fireEvent.click(powerControl);
    expect(priceControl).toBeDisabled();

    fireEvent.click(screen.getByRole("button", { name: "控制总线断开" }));
    fireEvent.click(screen.getByRole("button", { name: "排序方式: 最新" }));
    await waitFor(() => {
      expect((screen.getByLabelText("排序") as HTMLSelectElement).value).toBe(
        "priceAsc",
      );
    });

    fireEvent.click(screen.getByRole("button", { name: "筛选归零 执行" }));
    await waitFor(() => {
      expect(
        (screen.getByLabelText("资源状态") as HTMLSelectElement).value,
      ).toBe("");
      expect((screen.getByLabelText("排序") as HTMLSelectElement).value).toBe(
        "newest",
      );
    });
  });

  it("restores the document language from the saved locale", async () => {
    window.localStorage.setItem("gpu-rental-locale", "en");
    const gateway = new DemoGateway(new MemoryStorage());

    render(
      <MemoryRouter initialEntries={["/"]}>
        <AppProviders gateway={gateway}>
          <AppRoutes />
        </AppProviders>
      </MemoryRouter>,
    );

    await waitFor(() => {
      expect(document.documentElement.lang).toBe("en");
    });
    expect(
      await screen.findByRole("heading", {
        level: 1,
        name: /FIND A GPU/,
      }),
    ).toBeInTheDocument();
  });

  it("uses a browser-valid username pattern", async () => {
    const gateway = new DemoGateway(new MemoryStorage());

    render(
      <MemoryRouter initialEntries={["/register"]}>
        <AppProviders gateway={gateway}>
          <AppRoutes />
        </AppProviders>
      </MemoryRouter>,
    );

    const username = await screen.findByRole("textbox", { name: "用户名" });
    expect(username).toHaveAttribute("pattern", "[A-Za-z0-9_\\-]+");

    fireEvent.change(username, { target: { value: "invalid user" } });
    expect(username).toBeInvalid();
    fireEvent.change(username, { target: { value: "valid_user-1" } });
    expect(username).toBeValid();
  });
});
