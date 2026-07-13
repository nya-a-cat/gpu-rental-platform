import {
  GpuAvailability,
  type GpuResourceFacets,
  type GpuResourceView,
  type PaginatedResponse,
} from "@gpu-rental/contracts";
import { useEffect, useMemo, useState } from "react";
import { Link } from "react-router-dom";

import { readErrorMessage, useApp, useLocale } from "../app-context";
import nasaControlRoom from "../assets/archive/nasa-control-room-1976.jpg";
import inspectionCalibrationPlate from "../assets/generated/inspection-calibration-plate.webp";
import {
  AnalogGauge,
  EmptyState,
  ErrorState,
  LoadState,
  MechanicalPanel,
  Pagination,
  RotaryControl,
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

const AVAILABILITY_STOPS = [
  "",
  GpuAvailability.Available,
  GpuAvailability.Rented,
] as const;
const SORT_STOPS = ["newest", "priceAsc", "priceDesc"] as const;

function closestStopIndex(stops: number[], value: number): number {
  return stops.reduce(
    (closest, stop, index) =>
      Math.abs(stop - value) < Math.abs((stops[closest] ?? 0) - value)
        ? index
        : closest,
    0,
  );
}

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
  const [filtersExpanded, setFiltersExpanded] = useState(false);
  const [consoleArmed, setConsoleArmed] = useState(true);
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
  const priceStops = useMemo(() => {
    const ceiling = facets.maxHourlyPriceCents;
    if (!ceiling) return [0, 0, 0];
    return [
      ceiling,
      Math.max(100, Math.round((ceiling * 0.65) / 100) * 100),
      Math.max(100, Math.round((ceiling * 0.4) / 100) * 100),
    ];
  }, [facets.maxHourlyPriceCents]);
  const activeFilterCount = useMemo(
    () => [model, region, memoryGb, availability].filter(Boolean).length,
    [availability, memoryGb, model, region],
  );
  const availabilityPosition = AVAILABILITY_STOPS.indexOf(
    availability as (typeof AVAILABILITY_STOPS)[number],
  );
  const pricePosition = closestStopIndex(priceStops, maxPrice);
  const sortPosition = SORT_STOPS.indexOf(sort);

  const availabilityReadout = availability
    ? availabilityLabel(availability as GpuAvailability, tr)
    : tr("全部", "ALL");
  const sortReadout =
    sort === "newest"
      ? tr("最新", "NEWEST")
      : sort === "priceAsc"
        ? tr("低价", "LOW FIRST")
        : tr("高价", "HIGH FIRST");

  function resetFilters(): void {
    setModel("");
    setRegion("");
    setMemoryGb("");
    setAvailability("");
    setMaxPrice(facets.maxHourlyPriceCents);
    setSort("newest");
    setPage(1);
  }

  function cycleAvailability(): void {
    const next =
      AVAILABILITY_STOPS[
        (Math.max(0, availabilityPosition) + 1) % AVAILABILITY_STOPS.length
      ];
    setAvailability(next ?? "");
    setPage(1);
  }

  function cyclePrice(): void {
    if (!facets.maxHourlyPriceCents) return;
    setMaxPrice(
      priceStops[(pricePosition + 1) % priceStops.length] ??
        facets.maxHourlyPriceCents,
    );
    setPage(1);
  }

  function cycleSort(): void {
    setSort(SORT_STOPS[(sortPosition + 1) % SORT_STOPS.length] ?? "newest");
    setPage(1);
  }

  return (
    <div
      className="page-frame market-page"
      data-console-state={consoleArmed ? "armed" : "offline"}
    >
      <section className="market-hero">
        <div className="hero-copy">
          <span className="serial-label">KILOWORKS / GPU INVENTORY 2026</span>
          <p className="hero-kicker">
            {tr("GPU 资源租赁调度台", "GPU RESOURCE EXCHANGE")}
          </p>
          <h1>
            <span className="hero-title-part">
              {tr("筛 GPU。", "FIND A GPU.")}
            </span>{" "}
            <span className="hero-title-part">
              {tr("看价格。", "SEE THE PRICE.")}
            </span>
            <span className="hero-title-accent">
              {tr("直接预订。", "RESERVE IT.")}
            </span>
          </h1>
          <p>
            {tr(
              "按型号、显存、区域与小时价格筛选库存。确认资源后创建订单，需要时一键退租。",
              "Filter inventory by model, memory, region and hourly rate. Create an order, then return it when the work is done.",
            )}
          </p>
          <div className="hero-actions">
            <a className="button button--orange" href="#inventory-grid">
              {tr("进入资源列阵", "Browse inventory")}
            </a>
            <span className="plate-note">RESOURCE MODE / SIMULATED</span>
          </div>
          <div className="hero-readouts">
            <div>
              <span>{tr("本页可租", "AVAILABLE")}</span>
              <strong>{availableCount.toString().padStart(2, "0")}</strong>
            </div>
            <div>
              <span>{tr("覆盖区域", "REGIONS")}</span>
              <strong>
                {facets.regions.length.toString().padStart(2, "0")}
              </strong>
            </div>
            <div>
              <span>{tr("价格上限", "PRICE LIMIT")}</span>
              <strong>
                {maxPrice
                  ? formatMoney(maxPrice, locale).replace("CN¥", "¥")
                  : "—"}
              </strong>
            </div>
            <div>
              <span>{tr("订单路径", "ORDER PATH")}</span>
              <strong>{tr("预订 → 退租", "BOOK → RETURN")}</strong>
            </div>
          </div>
        </div>
        <figure className="hero-archive">
          <img
            alt={tr(
              "1976 年 NASA 控制室仪表墙档案照片",
              "Archival photograph of a 1976 NASA control-room instrument wall",
            )}
            decoding="async"
            referrerPolicy="no-referrer"
            src={nasaControlRoom}
          />
          <figcaption>
            <a
              href="https://commons.wikimedia.org/wiki/File:INSTRUMENT_PANELS_IN_CONTROL_ROOM_-_NARA_-_17447770.jpg"
              rel="noreferrer"
              target="_blank"
            >
              MARTIN BROWN / NASA / NARA
            </a>
            <span>PUBLIC DOMAIN (US) · ARCHIVE REFERENCE</span>
          </figcaption>
        </figure>
        <MechanicalPanel
          className={`gauge-console ${consoleArmed ? "is-armed" : "is-offline"}`}
          eyebrow="INVENTORY STATUS"
        >
          <div className="calibration-strip" aria-hidden="true">
            <img alt="" src={inspectionCalibrationPlate} />
            <span>SELECTOR CALIBRATION / 00—100</span>
          </div>
          <div
            aria-label={tr("快速控制台", "Quick control console")}
            className="console-action-rail"
          >
            <button
              aria-pressed={consoleArmed}
              className="console-action console-action--power"
              onClick={() => setConsoleArmed((value) => !value)}
              type="button"
            >
              <span aria-hidden="true" />
              <small>{tr("控制总线", "CONTROL BUS")}</small>
              <strong>
                {consoleArmed ? tr("接通", "ON") : tr("断开", "OFF")}
              </strong>
            </button>
            <button
              aria-pressed={availability === GpuAvailability.Available}
              className="console-action console-action--available"
              disabled={!consoleArmed}
              onClick={() => {
                setAvailability((current) =>
                  current === GpuAvailability.Available
                    ? ""
                    : GpuAvailability.Available,
                );
                setPage(1);
              }}
              type="button"
            >
              <span aria-hidden="true" />
              <small>{tr("可租锁定", "AVAILABLE LOCK")}</small>
              <strong>
                {availability === GpuAvailability.Available
                  ? tr("启用", "ACTIVE")
                  : tr("待机", "STANDBY")}
              </strong>
            </button>
            <button
              className="console-action console-action--reset"
              disabled={!consoleArmed}
              onClick={resetFilters}
              type="button"
            >
              <span aria-hidden="true" />
              <small>{tr("筛选归零", "RESET BANK")}</small>
              <strong>{tr("执行", "PRESS")}</strong>
            </button>
          </div>
          <div className="console-label-row">
            <strong>{tr("库存状态", "INVENTORY STATUS")}</strong>
            <StatusLamp
              label={
                consoleArmed
                  ? tr("控制台已接通", "CONSOLE ARMED")
                  : tr("控制台已断开", "CONSOLE OFFLINE")
              }
              tone={consoleArmed ? "good" : "neutral"}
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
          <div
            aria-label={tr("库存旋钮组", "Inventory rotary controls")}
            className="console-switches"
          >
            <RotaryControl
              disabled={!consoleArmed}
              label={tr("资源状态", "STATE")}
              onChange={cycleAvailability}
              position={Math.max(0, availabilityPosition)}
              value={availabilityReadout}
            />
            <RotaryControl
              disabled={!consoleArmed || !facets.maxHourlyPriceCents}
              label={tr("价格上限", "PRICE")}
              onChange={cyclePrice}
              position={pricePosition}
              value={
                maxPrice
                  ? formatMoney(maxPrice, locale).replace("CN¥", "¥")
                  : "—"
              }
            />
            <RotaryControl
              disabled={!consoleArmed}
              label={tr("排序方式", "SORT")}
              onChange={cycleSort}
              position={sortPosition}
              value={sortReadout}
            />
          </div>
        </MechanicalPanel>
      </section>

      <div className="signal-bridge" role="status">
        <span className="signal-bridge__bus">
          <i aria-hidden="true" />
          <small>{tr("控制总线", "CONTROL BUS")}</small>
          <strong>
            {consoleArmed ? tr("接通", "ARMED") : tr("断开", "OFFLINE")}
          </strong>
        </span>
        <span>
          <small>{tr("资源状态", "STATE")}</small>
          <strong>{availabilityReadout}</strong>
        </span>
        <span>
          <small>{tr("价格上限", "PRICE CEILING")}</small>
          <strong>
            {maxPrice ? formatMoney(maxPrice, locale).replace("CN¥", "¥") : "—"}
          </strong>
        </span>
        <span>
          <small>{tr("排序方式", "SORT")}</small>
          <strong>{sortReadout}</strong>
        </span>
        <a href="#inventory-grid">
          <small>{tr("库存机架", "INVENTORY RACK")}</small>
          <strong>{resources.total.toString().padStart(3, "0")} ↓</strong>
        </a>
      </div>

      <div className="market-workbench">
        <MechanicalPanel
          className={`filter-panel${filtersExpanded ? " is-expanded" : ""}`}
          eyebrow="FILTER BANK / A"
          title={tr("资源筛选器", "Resource selector")}
        >
          <button
            aria-expanded={filtersExpanded}
            className="filter-panel-toggle"
            onClick={() => setFiltersExpanded((value) => !value)}
            type="button"
          >
            {filtersExpanded
              ? tr("收起筛选条件", "Close filters")
              : tr(
                  `展开筛选 · ${activeFilterCount} 项已启用`,
                  `Open filters · ${activeFilterCount} active`,
                )}
          </button>
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
              step="10"
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
              <h2>{tr("可预订资源", "Bookable inventory")}</h2>
            </div>
            <span className="engraved-label">
              {tr("价格与状态来自库存记录", "PRICE & STATE FROM INVENTORY")}
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
    <article
      className={`resource-card${available ? " is-available" : " is-rented"}`}
    >
      <div className="resource-card__rail">
        <span className="card-index">
          {(index + 1).toString().padStart(2, "0")}
        </span>
        <StatusLamp
          label={availabilityLabel(resource.availability, tr)}
          tone={statusTone(resource.availability)}
        />
      </div>
      <div className="resource-card__identity">
        <div>
          <span>{resource.name}</span>
          <h3>{resource.model}</h3>
        </div>
        <div className="resource-schematic" aria-hidden="true">
          <span className="schematic-chip">GPU</span>
          <span className="schematic-bus" />
          {Array.from({ length: 4 }, (_, block) => (
            <span className="schematic-memory" key={block} />
          ))}
        </div>
        <div className="tag-row">
          {resource.tags.map((tag) => (
            <span key={tag}>{tag}</span>
          ))}
        </div>
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
          <dt>{tr("资源记录", "RECORD")}</dt>
          <dd>{resource.id.slice(-6).toUpperCase()}</dd>
        </div>
      </dl>
      <div className="resource-card__price">
        <span>{tr("小时单价", "HOURLY RATE")}</span>
        <strong>{formatMoney(resource.hourlyPriceCents, locale)}</strong>
        <small>/ HOUR</small>
        <Link
          className={`button ${available ? "button--orange" : "button--quiet"}`}
          to={`/resources/${resource.id}`}
        >
          {available
            ? tr("查看并预订", "Inspect & reserve")
            : tr("查看详情", "Inspect")}
        </Link>
      </div>
    </article>
  );
}
