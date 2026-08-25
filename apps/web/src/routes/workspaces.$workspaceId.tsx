import { createFileRoute, Outlet } from '@tanstack/react-router'

// This file is a pathless layout for /workspaces/$workspaceId/* — it exists
// only so TanStack Router's file-based routing recognizes the shared
// $workspaceId param segment. The actual pages are sibling routes:
// workspaces.$workspaceId.index.tsx (the tabbed detail view) and
// workspaces.$workspaceId.live.tsx (the live view grid) — each renders its
// own complete <AppShell>, so this layout must stay a bare <Outlet />
// rather than wrapping shared chrome, or they'd get double-nested shells.
export const Route = createFileRoute('/workspaces/$workspaceId')({
  component: () => <Outlet />,
})
