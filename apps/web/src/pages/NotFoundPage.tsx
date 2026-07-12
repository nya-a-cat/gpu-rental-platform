import { Link, useLocation } from "react-router-dom";

import { useLocale } from "../app-context";
import { EmptyState, MechanicalPanel } from "../components/mechanical";

export function NotFoundPage() {
  const { tr } = useLocale();
  const location = useLocation();
  const denied = location.pathname === "/denied";
  return (
    <div className="page-frame page-frame--narrow">
      <MechanicalPanel eyebrow="ROUTE RELAY / 404">
        <EmptyState
          action={
            <Link className="button button--orange" to="/">
              {tr("返回算力市场", "Return to market")}
            </Link>
          }
          message={
            denied
              ? tr(
                  "当前身份没有管理员面板访问权限。",
                  "Your current identity cannot access the admin panel.",
                )
              : tr(
                  "该线路不存在，或已经从控制台移除。",
                  "This route does not exist or has been removed from the console.",
                )
          }
          title={
            denied
              ? tr("访问被拒绝", "Access denied")
              : tr("未找到页面", "Page not found")
          }
        />
      </MechanicalPanel>
    </div>
  );
}
