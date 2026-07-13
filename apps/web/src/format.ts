export function formatMoney(cents: number, locale: "en" | "zh"): string {
  return new Intl.NumberFormat(locale === "zh" ? "zh-CN" : "en-US", {
    style: "currency",
    currency: "CNY",
    minimumFractionDigits: 2,
  }).format(cents / 100);
}

export function formatDate(value: string, locale: "en" | "zh"): string {
  return new Intl.DateTimeFormat(locale === "zh" ? "zh-CN" : "en-US", {
    dateStyle: "medium",
    timeStyle: "short",
  }).format(new Date(value));
}

export function clampPercentage(value: number): number {
  return Math.max(0, Math.min(100, value));
}

export function formatResourceRecord(resourceId: string): string {
  const demoRecord = resourceId.match(/^demo-(gpu-\d+)$/i)?.[1];
  return (demoRecord ?? resourceId.slice(-6)).toUpperCase();
}
