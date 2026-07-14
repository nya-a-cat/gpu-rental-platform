import {
  OrderStatus,
  type OrderView,
  type PaginatedResponse,
} from "@gpu-rental/contracts";
import { useEffect, useState } from "react";
import { Link } from "react-router-dom";

import { readErrorMessage, useApp, useLocale } from "../app-context";
import {
  EmptyState,
  ErrorState,
  LoadState,
  MechanicalPanel,
  MetricTile,
  Pagination,
  StatusLamp,
} from "../components/mechanical";
import { formatDate, formatMoney } from "../format";
import { orderStatusLabel, statusTone } from "../labels";

const EMPTY_ORDERS: PaginatedResponse<OrderView> = {
  items: [],
  page: 1,
  pageSize: 10,
  total: 0,
};

export function OrdersPage() {
  const { gateway, user } = useApp();
  const { locale, tr } = useLocale();
  const [orders, setOrders] = useState(EMPTY_ORDERS);
  const [status, setStatus] = useState("");
  const [page, setPage] = useState(1);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [pendingOrderId, setPendingOrderId] = useState<string | null>(null);
  const [revision, setRevision] = useState(0);

  useEffect(() => {
    let active = true;
    setLoading(true);
    setError(null);
    void gateway
      .listMyOrders({
        page,
        pageSize: 10,
        status: status ? (status as OrderStatus) : undefined,
      })
      .then((result) => {
        if (active) setOrders(result);
      })
      .catch((reason: unknown) => {
        if (active) setError(readErrorMessage(reason));
      })
      .finally(() => {
        if (active) setLoading(false);
      });
    return () => {
      active = false;
    };
  }, [gateway, page, revision, status]);

  async function returnOrder(orderId: string): Promise<void> {
    setPendingOrderId(orderId);
    setError(null);
    try {
      await gateway.returnOrder(orderId);
      setRevision((value) => value + 1);
    } catch (reason) {
      setError(readErrorMessage(reason));
    } finally {
      setPendingOrderId(null);
    }
  }

  const activeCount = orders.items.filter(
    (order) => order.status === OrderStatus.Active,
  ).length;
  const pageSpend = orders.items.reduce(
    (sum, order) => sum + order.totalPriceCents,
    0,
  );

  return (
    <div className="page-frame orders-page">
      <header className="page-title-row">
        <div>
          <span className="serial-label">ORDER DESK / PANEL 04</span>
          <h1>{tr("我的订单", "My orders")}</h1>
          <p>
            {tr(
              `当前操作员：${user?.username ?? "—"}。生效订单可以一键退租。`,
              `Current operator: ${user?.username ?? "—"}. Active orders can be returned in one step.`,
            )}
          </p>
        </div>
        <Link className="button button--orange" to="/">
          {tr("继续选择资源", "Find another resource")}
        </Link>
      </header>

      <div className="order-metrics">
        <MetricTile
          label={tr("筛选结果", "FILTERED ORDERS")}
          value={orders.total}
        />
        <MetricTile label={tr("本页生效", "ACTIVE HERE")} value={activeCount} />
        <MetricTile
          label={tr("本页预订金额", "BOOKED HERE")}
          value={formatMoney(pageSpend, locale)}
        />
      </div>

      <MechanicalPanel className="orders-console" eyebrow="ORDER REGISTER / A">
        <div className="toolbar">
          <label>
            <span>{tr("订单状态", "Order status")}</span>
            <select
              value={status}
              onChange={(event) => {
                setStatus(event.target.value);
                setPage(1);
              }}
            >
              <option value="">{tr("全部状态", "All states")}</option>
              {Object.values(OrderStatus).map((value) => (
                <option key={value} value={value}>
                  {orderStatusLabel(value, tr)}
                </option>
              ))}
            </select>
          </label>
          <span className="engraved-label">
            SESSION / {user?.role.toUpperCase()}
          </span>
        </div>

        {loading ? <LoadState /> : null}
        {!loading && error ? (
          <ErrorState
            message={error}
            retry={() => setRevision((value) => value + 1)}
          />
        ) : null}
        {!loading && !error && orders.items.length === 0 ? (
          <EmptyState
            action={
              <Link className="button button--orange" to="/">
                {tr("浏览资源", "Browse resources")}
              </Link>
            }
            message={tr(
              "当前条件下没有订单。新预订会出现在这里。",
              "There are no orders in this state. New reservations appear here.",
            )}
            title={tr("订单寄存器为空", "Order register empty")}
          />
        ) : null}
        {!loading && orders.items.length > 0 ? (
          <div className="order-list">
            {orders.items.map((order) => (
              <article className="order-card" key={order.id}>
                <div className="order-card__rail" aria-hidden="true">
                  <span />
                  <span />
                  <span />
                </div>
                <div className="order-card__main">
                  <div className="order-card__heading">
                    <div>
                      <span>{order.gpuName}</span>
                      <h2>{order.gpuModel}</h2>
                      <code>{order.id}</code>
                    </div>
                    <StatusLamp
                      label={orderStatusLabel(order.status, tr)}
                      tone={statusTone(order.status)}
                    />
                  </div>
                  <dl className="order-fields">
                    <div>
                      <dt>{tr("显存", "MEMORY")}</dt>
                      <dd>{order.gpuMemoryGb} GB</dd>
                    </div>
                    <div>
                      <dt>{tr("区域", "REGION")}</dt>
                      <dd>{order.region}</dd>
                    </div>
                    <div>
                      <dt>{tr("实例", "INSTANCE")}</dt>
                      <dd>{order.instanceName}</dd>
                    </div>
                    <div>
                      <dt>{tr("环境", "ENVIRONMENT")}</dt>
                      <dd>{order.environmentTemplateName}</dd>
                    </div>
                    <div>
                      <dt>{tr("成本归属", "COST ATTRIBUTION")}</dt>
                      <dd>
                        {order.projectName
                          ? `${order.teamName} / ${order.projectName}`
                          : tr("个人账户", "Personal account")}
                      </dd>
                    </div>
                    <div>
                      <dt>{tr("租期", "DURATION")}</dt>
                      <dd>{order.durationHours} h</dd>
                    </div>
                    <div>
                      <dt>{tr("总价", "TOTAL")}</dt>
                      <dd>{formatMoney(order.totalPriceCents, locale)}</dd>
                    </div>
                    <div>
                      <dt>{tr("开始", "START")}</dt>
                      <dd>{formatDate(order.startsAt, locale)}</dd>
                    </div>
                    <div>
                      <dt>{tr("结束", "END")}</dt>
                      <dd>{formatDate(order.endsAt, locale)}</dd>
                    </div>
                  </dl>
                </div>
                <div className="order-card__action">
                  {order.status === OrderStatus.Active ? (
                    <button
                      className="button button--orange"
                      disabled={pendingOrderId === order.id}
                      onClick={() => void returnOrder(order.id)}
                      type="button"
                    >
                      {pendingOrderId === order.id
                        ? tr("正在退租…", "Returning…")
                        : tr("一键退租", "Return now")}
                    </button>
                  ) : (
                    <span className="terminal-stamp">TERMINAL</span>
                  )}
                </div>
              </article>
            ))}
          </div>
        ) : null}
        <Pagination
          onChange={setPage}
          page={orders.page}
          pageSize={orders.pageSize}
          total={orders.total}
        />
      </MechanicalPanel>
    </div>
  );
}
