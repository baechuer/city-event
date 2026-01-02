import { useState } from "react";
import { useAuth } from "@/lib/auth";
import { apiClient } from "@/lib/apiClient";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { toast } from "sonner";
import { Shield, Lock, Mail, Fingerprint, Eye, EyeOff, CheckCircle2, XCircle } from "lucide-react";

export function Profile() {
    const { user } = useAuth();
    const [loading, setLoading] = useState(false);
    const [showPasswords, setShowPasswords] = useState({
        old: false,
        new: false,
        confirm: false,
    });
    const [passwordData, setPasswordData] = useState({
        old_password: "",
        new_password: "",
        confirm_password: "",
    });

    const toggleVisibility = (field: keyof typeof showPasswords) => {
        setShowPasswords(prev => ({ ...prev, [field]: !prev[field] }));
    };

    const handleChangePassword = async (e: React.FormEvent) => {
        e.preventDefault();

        if (passwordData.new_password !== passwordData.confirm_password) {
            toast.error("New passwords do not match");
            return;
        }

        if (passwordData.new_password.length < 12) {
            toast.error("New password must be at least 12 characters");
            return;
        }

        setLoading(true);
        try {
            await apiClient.post("/auth/password/change", {
                old_password: passwordData.old_password,
                new_password: passwordData.new_password,
            });
            toast.success("Password changed successfully");
            setPasswordData({
                old_password: "",
                new_password: "",
                confirm_password: "",
            });
        } catch (err: any) {
            toast.error(err.message || "Failed to change password");
        } finally {
            setLoading(false);
        }
    };

    return (
        <main className="container mx-auto py-12 px-4 max-w-4xl">
            <div className="mb-8">
                <h1 className="text-4xl font-extrabold tracking-tight text-slate-900 dark:text-white">
                    Account <span className="text-emerald-600">Settings</span>
                </h1>
                <p className="text-muted-foreground mt-2">
                    Manage your profile information and security preferences.
                </p>
            </div>

            <div className="grid grid-cols-1 md:grid-cols-3 gap-8">
                {/* Profile Info Card */}
                <div className="md:col-span-1">
                    <div className="glass-card p-6 rounded-3xl relative overflow-hidden group border-white/20 backdrop-blur-xl h-full flex flex-col">
                        <div className="absolute top-0 right-0 w-32 h-32 bg-emerald-600/5 rounded-full -mr-16 -mt-16 transition-transform group-hover:scale-110" />

                        <div className="relative flex-1 flex flex-col">
                            <div className="w-20 h-20 rounded-2xl bg-emerald-600 flex items-center justify-center text-white text-3xl font-bold shadow-lg mb-6 shadow-emerald-500/20">
                                {(user?.name?.[0] || user?.email?.[0] || 'U').toUpperCase()}
                            </div>

                            <div className="space-y-6 flex-1">
                                <div>
                                    <Label className="text-[10px] uppercase tracking-widest text-muted-foreground font-bold flex items-center gap-1.5 mb-1.5">
                                        <Mail className="w-3 h-3" /> Email Address
                                    </Label>
                                    <p className="text-sm font-semibold truncate text-slate-700 dark:text-slate-200">{user?.email}</p>
                                </div>

                                <div>
                                    <Label className="text-[10px] uppercase tracking-widest text-muted-foreground font-bold flex items-center gap-1.5 mb-1.5">
                                        <Shield className="w-3 h-3" /> Account Role
                                    </Label>
                                    <span className="inline-flex items-center px-2 py-0.5 rounded-full text-[10px] font-bold bg-emerald-100 text-emerald-700 dark:bg-emerald-950/30 dark:text-emerald-400 uppercase tracking-tighter shadow-sm">
                                        {user?.role || 'User'}
                                    </span>
                                </div>

                                <div>
                                    <Label className="text-[10px] uppercase tracking-widest text-muted-foreground font-bold flex items-center gap-1.5 mb-1.5">
                                        <Shield className="w-3 h-3" /> Verification Status
                                    </Label>
                                    {user?.email_verified ? (
                                        <span className="inline-flex items-center gap-1.5 px-2 py-0.5 rounded-full text-[10px] font-bold bg-emerald-50 text-emerald-600 dark:bg-emerald-950/30 dark:text-emerald-400 uppercase tracking-tighter shadow-sm border border-emerald-100 dark:border-emerald-900">
                                            <CheckCircle2 className="w-2.5 h-2.5" /> Verified
                                        </span>
                                    ) : (
                                        <span className="inline-flex items-center gap-1.5 px-2 py-0.5 rounded-full text-[10px] font-bold bg-amber-50 text-amber-600 dark:bg-amber-950/30 dark:text-amber-400 uppercase tracking-tighter shadow-sm border border-amber-100 dark:border-amber-900">
                                            <XCircle className="w-2.5 h-2.5" /> Unverified
                                        </span>
                                    )}
                                </div>

                                <div className="mt-auto pt-6 border-t border-slate-100 dark:border-white/5">
                                    <Label className="text-[10px] uppercase tracking-widest text-muted-foreground font-bold flex items-center gap-1.5 mb-1.5">
                                        <Fingerprint className="w-3 h-3" /> Unique ID
                                    </Label>
                                    <p className="text-[10px] font-mono text-muted-foreground truncate opacity-70">{user?.id}</p>
                                </div>
                            </div>
                        </div>
                    </div>
                </div>

                {/* Security Section */}
                <div className="md:col-span-2">
                    <div className="glass-card p-8 rounded-3xl border-white/20 backdrop-blur-xl shadow-xl shadow-slate-200/50 dark:shadow-none h-full flex flex-col">
                        <div className="flex items-center gap-3 mb-8">
                            <div className="w-10 h-10 rounded-xl bg-slate-100 dark:bg-slate-800 flex items-center justify-center text-slate-600 dark:text-slate-400 shadow-sm">
                                <Lock className="w-5 h-5" />
                            </div>
                            <div>
                                <h3 className="text-xl font-bold text-slate-900 dark:text-white">Security</h3>
                                <p className="text-sm text-muted-foreground">Update your password to keep your account safe.</p>
                            </div>
                        </div>

                        <form onSubmit={handleChangePassword} className="space-y-6">
                            <div className="space-y-2">
                                <Label htmlFor="old_password">Current Password</Label>
                                <div className="relative">
                                    <Input
                                        id="old_password"
                                        type={showPasswords.old ? "text" : "password"}
                                        placeholder="Enter current password"
                                        className="bg-white/50 dark:bg-slate-900/50 pr-10 border-white/30"
                                        value={passwordData.old_password}
                                        onChange={(e) => setPasswordData(prev => ({ ...prev, old_password: e.target.value }))}
                                        required
                                    />
                                    <button
                                        type="button"
                                        onClick={() => toggleVisibility('old')}
                                        className="absolute right-3 top-1/2 -translate-y-1/2 text-muted-foreground hover:text-slate-900 dark:hover:text-white transition-colors p-1"
                                    >
                                        {showPasswords.old ? <EyeOff className="w-4 h-4" /> : <Eye className="w-4 h-4" />}
                                    </button>
                                </div>
                            </div>

                            <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
                                <div className="space-y-2">
                                    <Label htmlFor="new_password">New Password</Label>
                                    <div className="relative">
                                        <Input
                                            id="new_password"
                                            type={showPasswords.new ? "text" : "password"}
                                            placeholder="Min 12 characters"
                                            className="bg-white/50 dark:bg-slate-900/50 pr-10 border-white/30"
                                            value={passwordData.new_password}
                                            onChange={(e) => setPasswordData(prev => ({ ...prev, new_password: e.target.value }))}
                                            required
                                        />
                                        <button
                                            type="button"
                                            onClick={() => toggleVisibility('new')}
                                            className="absolute right-3 top-1/2 -translate-y-1/2 text-muted-foreground hover:text-slate-900 dark:hover:text-white transition-colors p-1"
                                        >
                                            {showPasswords.new ? <EyeOff className="w-4 h-4" /> : <Eye className="w-4 h-4" />}
                                        </button>
                                    </div>
                                </div>
                                <div className="space-y-2">
                                    <Label htmlFor="confirm_password">Confirm New Password</Label>
                                    <div className="relative">
                                        <Input
                                            id="confirm_password"
                                            type={showPasswords.confirm ? "text" : "password"}
                                            placeholder="Repeat new password"
                                            className="bg-white/50 dark:bg-slate-900/50 pr-10 border-white/30"
                                            value={passwordData.confirm_password}
                                            onChange={(e) => setPasswordData(prev => ({ ...prev, confirm_password: e.target.value }))}
                                            required
                                        />
                                        <button
                                            type="button"
                                            onClick={() => toggleVisibility('confirm')}
                                            className="absolute right-3 top-1/2 -translate-y-1/2 text-muted-foreground hover:text-slate-900 dark:hover:text-white transition-colors p-1"
                                        >
                                            {showPasswords.confirm ? <EyeOff className="w-4 h-4" /> : <Eye className="w-4 h-4" />}
                                        </button>
                                    </div>
                                </div>
                            </div>

                            <div className="pt-4 flex justify-end">
                                <Button
                                    type="submit"
                                    disabled={loading}
                                    className="rounded-full px-8 bg-emerald-600 hover:bg-emerald-700 shadow-lg shadow-emerald-500/20 active:scale-95 transition-transform"
                                >
                                    {loading ? "Updating..." : "Change Password"}
                                </Button>
                            </div>
                        </form>
                    </div>
                </div>
            </div>
        </main>
    );
}
