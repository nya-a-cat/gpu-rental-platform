import {
  GpuAvailability,
  type GpuResourceFacets,
  type GpuResourceView,
  type PaginatedResponse,
} from "@gpu-rental/contracts";
import { useEffect, useMemo, useState } from "react";
import { Link } from "react-router-dom";

import { readErrorMessage, useApp, useLocale } from "../app-context";
import mechanicalStatusPlate from "../assets/generated/mechanical-status-plate.webp";
import {
  AnalogGauge,
  EmptyState,
  ErrorState,
  LoadState,
  MechanicalPanel,
  Pagination,
  RotaryMark,
  StatusLamp,
} from "../components/mechanical";
import { formatMoney } from "../format";
import { availabilityLabel, statusTone } from "../labels";

const EMPTY_FACETS: GpuResourceFacets = {
  models: [],
  regions: [],
  memoryGbValues: [],
  maxHourlyPriceCents: 0,
};

const EMPTY_PAGE: PaginatedResponse<GpuResourceView> = {
  items: [],
  page: 1,
  pageSize: 12,
  total: 0,
};

export function MarketPage() {
  const { gateway } = useApp();
  const { locale, tr } = useLocale();
  const [facets, setFacets] = useState(EMPTY_FACETS);
  const [resources, setResources] = useState(EMPTY_PAGE);
  const [model, setModel] = useState("");
  const [region, setRegion] = useState("");
  const [memoryGb, setMemoryGb] = useState("");
  const [availability, setAvailability] = useState("");
  const [maxPrice, setMaxPrice] = useState(0);
  const [sort, setSort] = useState<"newest" | "priceAsc" | "priceDesc">(
    "newest",
  );
  const [page, setPage] = useState(1);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [revision, setRevision] = useState(0);

  useEffect(() => {
    let active = true;
    void gateway
      .getFacets()
      .then((nextFacets) => {
        if (!active) return;
        setFacets(nextFacets);
        setMaxPrice((current) => current || nextFacets.maxHourlyPriceCents);
      })
      .catch((reason: unknown) => {
        if (active) setError(readErrorMessage(reason));
      });
    return () => {
      active = false;
    };
  }, [gateway]);

  useEffect(() => {
    let active = true;
    setLoading(true);
    setError(null);
    void gateway
      .listResources({
        page,
        pageSize: 12,
        model: model || undefined,
        region: region || undefined,
        memoryGb: memoryGb ? Number(memoryGb) : undefined,
        availability: availability
          ? (availability as GpuAvailability)
          : undefined,
        maxHourlyPriceCents: maxPrice || undefined,
        sort,
      })
      .then((result) => {
        if (active) setResources(result);
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
  }, [
    availability,
    gateway,
    maxPrice,
    memoryGb,
    model,
    page,
    region,
    revision,
    sort,
  ]);

  const availableCount = resources.items.filter(
    (resource) => resource.availability === GpuAvailability.Available,
  ).length;
  const availabilityRatio = resources.items.length
    ? (availableCount / resources.items.length) * 100
    : 0;
  const priceRatio = facets.maxHourlyPriceCents
    ? (maxPrice / facets.maxHourlyPriceCents) * 100
    : 0;
  const modelRatio = Math.min(100, facets.models.length * 14);
  const activeFilterCount = useMemo(
    () => [model, region, memoryGb, availability].filter(Boolean).length,
    [availability, memoryGb, model, region],
  );

  function resetFilters(): void {
    setModel("");
    setRegion("");
    setMemoryGb("");
    setAvailability("");
    setMaxPrice(facets.maxHourlyPriceCents);
    setSort("newest");
    setPage(1);
  }

  return (
    <div className="page-frame market-page">
      <section className="market-hero">
        <div className="hero-copy">
          <span className="serial-label">KW-EXCHANGE / PANEL 01</span>
          <p className="hero-kicker">
            {tr("按需算力调度台", "ON-DEMAND COMPUTE DISPATCH")}
          </p>
          <h1>
            {tr("让每一块算力", "Put every compute unit")}
            <span>{tr("进入正确队列。", "in the right queue.")}</span>
          </h1>
          <p>
            {tr(
              "筛选模拟 GPU 资产，完成预订、订单流转与退租。公开演示不连接实体设备，也不展示伪造遥测。",
              "Filter simulated GPU assets and walk through reservation, order transitions and returns. The public demo never connects to hardware or invents telemetry.",
            )}
          </p>
          <div className="hero-actions">
            <a className="button button--orange" href="#inventory-grid">
              {tr("进入资源列阵", "Browse inventory")}
            </a>
            <span className="plate-note">RESOURCE MODE / SIMULATED</span>
          </div>
        </div>
        <MechanicalPanel className="gauge-console" eyebrow="MARKET READOUT">
          <img
            alt=""
            aria-hidden="true"
            className="console-art"
            src={mechanicalStatusPlate}
          />
          <div className="console-label-row">
            <strong>{tr("市场读数", "MARKET READOUT")}</strong>
            <StatusLamp
              label={tr("清单已连接", "LIST CONNECTED")}
              tone="good"
            />
          </div>
          <div className="gauge-row">
            <AnalogGauge
              display={`${availableCount}/${resources.items.length}`}
              label={tr("当前页可租", "AVAILABLE HERE")}
              value={availabilityRatio}
            />
            <AnalogGauge
              display={
                maxPrice
                  ? formatMoney(maxPrice, locale).replace("CN¥", "¥")
                  : "—"
              }
              label={tr("价格筛选上限", "PRICE CEILING")}
              value={priceRatio}
            />
            <AnalogGauge
              display={String(facets.models.length).padStart(2, "0")}
              label={tr("可选型号", "MODEL OPTIONS")}
              value={modelRatio}
            />
          </div>
          <div className="console-switches" aria-hidden="true">
            <RotaryMark active />
            <RotaryMark />
            <RotaryMark active />
            <span>CATALOG / ORDER / RETURN</span>
          </div>
        </MechanicalPanel>
      </section>

      <div className="market-workbench">
        <MechanicalPanel
          className="filter-panel"
          eyebrow="FILTER BANK / A"
          title={tr("资源筛选器", "Resource selector")}
        >
          <div className="filter-count">
            <span>{tr("已启用条件", "ACTIVE FILTERS")}</span>
            <strong>{activeFilterCount.toString().padStart(2, "0")}</strong>
          </div>
          <label>
            <span>{tr("GPU 型号", "GPU model")}</span>
            <select
              value={model}
              onChange={(event) => {
                setModel(event.target.value);
                setPage(1);
              }}
            >
              <option value="">{tr("全部型号", "All models")}</option>
              {facets.models.map((item) => (
                <option key={item} value={item}>
                  {item}
                </option>
              ))}
            </select>
          </label>
          <label>
            <span>{tr("区域", "Region")}</span>
            <select
              value={region}
              onChange={(event) => {
                setRegion(event.target.value);
                setPage(1);
              }}
            >
              <option value="">{tr("全部区域", "All regions")}</option>
              {facets.regions.map((item) => (
                <option key={item} value={item}>
                  {item}
                </option>
              ))}
            </select>
          </label>
          <label>
            <span>{tr("显存容量", "Memory")}</span>
            <select
              value={memoryGb}
              onChange={(event) => {
                setMemoryGb(event.target.value);
                setPage(1);
              }}
            >
              <option value="">{tr("全部容量", "All capacities")}</option>
              {facets.memoryGbValues.map((item) => (
                <option key={item} value={item}>
                  {item} GB
                </option>
              ))}
            </select>
          </label>
          <label>
            <span>{tr("资源状态", "Availability")}</span>
            <select
              value={availability}
              onChange={(event) => {
                setAvailability(event.target.value);
                setPage(1);
              }}
            >
              <option value="">{tr("全部状态", "All states")}</option>
              <option value={GpuAvailability.Available}>
                {tr("可预订", "Available")}
              </option>
              <option value={GpuAvailability.Rented}>
                {tr("租用中", "Rented")}
              </option>
            </select>
          </label>
          <label className="range-control">
            <span>
              {tr("每小时价格上限", "Hourly price ceiling")}
              <strong>
                {maxPrice
                  ? formatMoney(maxPrice, locale).replace("CN¥", "¥")
                  : "—"}
              </strong>
            </span>
            <input
              disabled={!facets.maxHourlyPriceCents}
              max={facets.maxHourlyPriceCents || 1}
              min="0"
              onChange={(event) => {
                setMaxPrice(Number(event.target.value));
                setPage(1);
              }}
              step="100"
              type="range"
              value={maxPrice}
            />
          </label>
          <label>
            <span>{tr("排序", "Sort")}</span>
            <select
              value={sort}
              onChange={(event) => {
                setSort(event.target.value as typeof sort);
                setPage(1);
              }}
            >
              <option value="newest">{tr("最新入列", "Newest")}</option>
              <option value="priceAsc">
                {tr("价格从低到高", "Price: low to high")}
              </option>
              <option value="priceDesc">
                {tr("价格从高到低", "Price: high to low")}
              </option>
            </select>
          </label>
          <button
            className="button button--dark"
            onClick={resetFilters}
            type="button"
          >
            {tr("归零筛选器", "Reset selector")}
          </button>
        </MechanicalPanel>

        <section className="inventory-section" id="inventory-grid">
          <header className="section-heading">
            <div>
              <span>
                INVENTORY / {resources.total.toString().padStart(3, "0")}
              </span>
              <h2>{tr("模拟资源列阵", "Simulated inventory")}</h2>
            </div>
            <span className="engraved-label">
              {tr("不含实时温度或利用率", "NO LIVE TELEMETRY")}
            </span>
          </header>

          {loading ? <LoadState /> : null}
          {!loading && error ? (
            <ErrorState
              message={error}
              retry={() => setRevision((value) => value + 1)}
            />
          ) : null}
          {!loading && !error && resources.items.length === 0 ? (
            <EmptyState
              action={
                <button
                  className="button button--orange"
                  onClick={resetFilters}
                  type="button"
                >
                  {tr("清除筛选", "Clear filters")}
                </button>
              }
              message={tr(
                "当前筛选条件下没有资源，调整价格或型号后重试。",
                "No resources match this selector. Adjust price or model and retry.",
              )}
              title={tr("列阵为空", "Inventory bank empty")}
            />
          ) : null}
          {!loading && !error && resources.items.length > 0 ? (
            <div className="resource-grid">
              {resources.items.map((resource, index) => (
                <ResourceCard
                  index={index}
                  key={resource.id}
                  resource={resource}
                />
              ))}
            </div>
          ) : null}
          <Pagination
            onChange={setPage}
            page={resources.page}
            pageSize={resources.pageSize}
            total={resources.total}
          />
        </section>
      </div>
    </div>
  );
}

function ResourceCard({
  index,
  resource,
}: {
  index: number;
  resource: GpuResourceView;
}) {
  const { locale, tr } = useLocale();
  const available = resource.availability === GpuAvailability.Available;
  return (
    <article className="resource-card">
      <span className="card-index">
        UNIT {(index + 1).toString().padStart(2, "0")}
      </span>
      <div className="resource-schematic" aria-hidden="true">
        <span className="schematic-chip">GPU</span>
        <span className="schematic-bus" />
        {Array.from({ length: 4 }, (_, block) => (
          <span className="schematic-memory" key={block} />
        ))}
      </div>
      <div className="resource-card__title">
        <div>
          <span>{resource.name}</span>
          <h3>{resource.model}</h3>
        </div>
        <StatusLamp
          label={availabilityLabel(resource.availability, tr)}
          tone={statusTone(resource.availability)}
        />
      </div>
      <dl className="spec-grid">
        <div>
          <dt>{tr("显存", "MEMORY")}</dt>
          <dd>{resource.memoryGb} GB</dd>
        </div>
        <div>
          <dt>{tr("区域", "REGION")}</dt>
          <dd>{resource.region}</dd>
        </div>
        <div>
          <dt>{tr("计费", "RATE")}</dt>
          <dd>{formatMoney(resource.hourlyPriceCents, locale)} / h</dd>
        </div>
        <div>
          <dt>{tr("资产", "MODE")}</dt>
          <dd>SIMULATED</dd>
        </div>
      </dl>
      <div className="tag-row">
        {resource.tags.map((tag) => (
          <span key={tag}>{tag}</span>
        ))}
      </div>
      <Link
        className={`button ${available ? "button--orange" : "button--quiet"}`}
        to={`/resources/${resource.id}`}
      >
        {available
          ? tr("查看并预订", "Inspect & reserve")
          : tr("查看详情", "Inspect")}
      </Link>
    </article>
  );
}
