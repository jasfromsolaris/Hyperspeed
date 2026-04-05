package rbac

// Permission is a string identifier checked by RBAC.
type Permission string

const (
	OrgManage        Permission = "org.manage"
	OrgMembersManage Permission = "org.members.manage"

	SpaceCreate        Permission = "space.create"
	SpaceDelete        Permission = "space.delete"
	SpaceMembersManage Permission = "space.members.manage"

	BoardRead  Permission = "board.read"
	BoardWrite Permission = "board.write"

	TasksRead  Permission = "tasks.read"
	TasksWrite Permission = "tasks.write"

	ChatRead  Permission = "chat.read"
	ChatWrite Permission = "chat.write"

	FilesRead   Permission = "files.read"
	FilesWrite  Permission = "files.write"
	FilesDelete Permission = "files.delete"

	TerminalUse         Permission = "terminal.use"
	SSHConnectionsManage Permission = "ssh_connections.manage"

	AgentToolsInvoke Permission = "agent.tools.invoke"

	DatasetsRead  Permission = "datasets.read"
	DatasetsWrite Permission = "datasets.write"
)

var AllPermissions = []Permission{
	OrgManage,
	OrgMembersManage,
	SpaceCreate,
	SpaceDelete,
	SpaceMembersManage,
	BoardRead,
	BoardWrite,
	TasksRead,
	TasksWrite,
	ChatRead,
	ChatWrite,
	FilesRead,
	FilesWrite,
	FilesDelete,
	TerminalUse,
	SSHConnectionsManage,
	AgentToolsInvoke,
	DatasetsRead,
	DatasetsWrite,
}

