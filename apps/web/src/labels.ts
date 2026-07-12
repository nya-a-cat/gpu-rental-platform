import {
  GpuAvailability,
  GpuListingStatus,
  OrderStatus,
} from "@gpu-rental/contracts";

export function availabilityLabel(
  value: GpuAvailability,
  tr: (zh: string, en: string) => string,
): string {
  return value === GpuAvailability.Available
    ? tr("可预订", "Available")
    : tr("租用中", "Rented");
}

export function listingLabel(
  value: GpuListingStatus,
  tr: (zh: string, en: string) => string,
): string {
  const labels: Record<GpuListingStatus, string> = {
    [GpuListingStatus.Online]: tr("已上架", "Online"),
    [GpuListingStatus.Offline]: tr("已下架", "Offline"),
    [GpuListingStatus.Maintenance]: tr("维护中", "Maintenance"),
  };
  return labels[value];
}

export function orderStatusLabel(
  value: OrderStatus,
  tr: (zh: string, en: string) => string,
): string {
  const labels: Record<OrderStatus, string> = {
    [OrderStatus.Active]: tr("生效中", "Active"),
    [OrderStatus.Returned]: tr("已退租", "Returned"),
    [OrderStatus.Expired]: tr("已到期", "Expired"),
    [OrderStatus.Cancelled]: tr("已取消", "Cancelled"),
  };
  return labels[value];
}

export function statusTone(
  value: string,
): "danger" | "good" | "neutral" | "warn" {
  if (
    value === GpuAvailability.Available ||
    value === GpuListingStatus.Online ||
    value === OrderStatus.Active
  ) {
    return "good";
  }
  if (value === GpuListingStatus.Maintenance) return "warn";
  if (value === OrderStatus.Cancelled) return "danger";
  return "neutral";
}
