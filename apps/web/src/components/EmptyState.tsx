interface EmptyStateProps {
    title?: string;
    description?: string;
    action?: React.ReactNode;
}

export function EmptyState({
    title = "No items found",
    description = "Check back later for new content.",
    action
}: EmptyStateProps) {
    return (
        <div className="flex flex-col items-center justify-center p-12 text-center space-y-4 border-2 border-dashed rounded-lg h-[40vh] text-muted-foreground">
            <div className="space-y-2">
                <h3 className="text-lg font-medium text-foreground">{title}</h3>
                <p className="text-sm">{description}</p>
            </div>
            {action && <div>{action}</div>}
        </div>
    );
}
