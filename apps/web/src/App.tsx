import { Navigate, Route, Routes } from "react-router-dom";
import { AppLayout } from "./layout/AppLayout";
import { ProtectedRoute } from "./layout/ProtectedRoute";
import LoginPage from "./pages/LoginPage";
import DashboardPage from "./pages/DashboardPage";
import InboxPage from "./pages/InboxPage";
import PeekPage from "./pages/PeekPage";
import OrgProjectsPage from "./pages/OrgProjectsPage";
import OrgRolesPage from "./pages/OrgRolesPage";
import OrgSettingsPage from "./pages/OrgSettingsPage";
import OrgSpacesAccessPage from "./pages/OrgSpacesAccessPage";
import OrgServiceAccountsPage from "./pages/OrgServiceAccountsPage";
import AcceptInvitePage from "./pages/AcceptInvitePage";
import ChatRoomPage from "./pages/ChatRoomPage";
import ProjectBoardPage from "./pages/ProjectBoardPage";
import SpaceDatasetsPage from "./pages/SpaceDatasetsPage";
import SpaceFilesPage from "./pages/SpaceFilesPage";
import SpaceIDEPage from "./pages/SpaceIDEPage";
import SpaceTerminalPage from "./pages/SpaceTerminalPage";
import SpaceAutomationsPage from "./pages/SpaceAutomationsPage";
import SpaceHomeRedirect from "./pages/SpaceHomeRedirect";
import RegisterPage from "./pages/RegisterPage";
import SignupPendingPage from "./pages/SignupPendingPage";
import { PendingBlocksApp } from "./layout/PendingBlocksApp";

export default function App() {
  return (
    <Routes>
      <Route path="/login" element={<LoginPage />} />
      <Route path="/register" element={<RegisterPage />} />
      <Route path="/invite/accept" element={<AcceptInvitePage />} />
      <Route element={<ProtectedRoute />}>
        <Route path="/signup-pending" element={<SignupPendingPage />} />
        <Route element={<PendingBlocksApp />}>
        <Route path="/" element={<AppLayout />}>
          <Route index element={<DashboardPage />} />
          <Route path="peek" element={<PeekPage />} />
          <Route path="inbox/peek" element={<Navigate to="/peek" replace />} />
          <Route path="inbox" element={<InboxPage />} />
          <Route path="o/:orgId" element={<OrgProjectsPage />} />
          <Route path="o/:orgId/roles" element={<OrgRolesPage />} />
          <Route path="o/:orgId/settings" element={<OrgSettingsPage />} />
          <Route path="o/:orgId/settings/spaces" element={<OrgSpacesAccessPage />} />
          <Route path="o/:orgId/settings/service-accounts" element={<OrgServiceAccountsPage />} />
          <Route path="o/:orgId/p/:projectId/ide" element={<SpaceIDEPage />} />
          <Route path="o/:orgId/p/:projectId/files" element={<SpaceFilesPage />} />
          <Route path="o/:orgId/p/:projectId/datasets" element={<SpaceDatasetsPage />} />
          <Route path="o/:orgId/p/:projectId/terminal" element={<SpaceTerminalPage />} />
          <Route path="o/:orgId/p/:projectId/automations" element={<SpaceAutomationsPage />} />
          <Route path="o/:orgId/p/:projectId/c/:chatRoomId" element={<ChatRoomPage />} />
          <Route path="o/:orgId/p/:projectId/b/:boardId" element={<ProjectBoardPage />} />
          <Route path="o/:orgId/p/:projectId" element={<SpaceHomeRedirect />} />
        </Route>
        </Route>
      </Route>
      <Route path="*" element={<Navigate to="/" replace />} />
    </Routes>
  );
}
