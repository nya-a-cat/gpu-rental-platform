import { UserRole } from "@gpu-rental/contracts";
import type { PropsWithChildren } from "react";
import {
  BrowserRouter,
  HashRouter,
  Navigate,
  Route,
  Routes,
  useLocation,
} from "react-router-dom";

import { AppProviders, useApp, useLocale } from "./app-context";
import { AppLayout } from "./components/layout";
import { LoadState } from "./components/mechanical";
import type { DataGateway } from "./data/gateway";
import { resolveRuntimeMode } from "./data/gateway";
import { AdminPage } from "./pages/AdminPage";
import { AuthPage } from "./pages/AuthPage";
import { MarketPage } from "./pages/MarketPage";
import { InstancesPage } from "./pages/InstancesPage";
import { NotFoundPage } from "./pages/NotFoundPage";
import { OrdersPage } from "./pages/OrdersPage";
import { ResourcePage } from "./pages/ResourcePage";

export function App({ gateway }: { gateway?: DataGateway }) {
  const routes = (
    <AppProviders gateway={gateway}>
      <AppRoutes />
    </AppProviders>
  );

  if (resolveRuntimeMode() === "demo") return <HashRouter>{routes}</HashRouter>;
  const basename = import.meta.env.BASE_URL.replace(/\/$/, "") || undefined;
  return <BrowserRouter basename={basename}>{routes}</BrowserRouter>;
}

export function AppRoutes() {
  return (
    <Routes>
      <Route element={<AppLayout />}>
        <Route index element={<MarketPage />} />
        <Route path="resources/:resourceId" element={<ResourcePage />} />
        <Route path="login" element={<AuthPage kind="login" />} />
        <Route path="register" element={<AuthPage kind="register" />} />
        <Route
          path="instances"
          element={
            <ProtectedRoute>
              <InstancesPage />
            </ProtectedRoute>
          }
        />
        <Route
          path="orders"
          element={
            <ProtectedRoute>
              <OrdersPage />
            </ProtectedRoute>
          }
        />
        <Route
          path="admin"
          element={
            <ProtectedRoute admin>
              <AdminPage />
            </ProtectedRoute>
          }
        />
        <Route path="*" element={<NotFoundPage />} />
      </Route>
    </Routes>
  );
}

function ProtectedRoute({
  admin = false,
  children,
}: PropsWithChildren<{ admin?: boolean }>) {
  const { sessionLoading, user } = useApp();
  const { tr } = useLocale();
  const location = useLocation();

  if (sessionLoading) {
    return (
      <div className="page-frame page-frame--narrow">
        <LoadState label={tr("正在核验访问权限", "Checking access relay")} />
      </div>
    );
  }
  if (!user) {
    return <Navigate replace state={{ from: location.pathname }} to="/login" />;
  }
  if (admin && user.role !== UserRole.Admin) {
    return <Navigate replace to="/denied" />;
  }
  return children;
}
