import { useState } from "react";
import { useNavigate } from "react-router-dom";
import { apiClient } from "@/lib/apiClient";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/Textarea";
import { toast } from "sonner";
import {
    Calendar,
    MapPin,
    Tag,
    Users,
    Type,
    AlignLeft,
    Clock,
    Send,
    Save,
    ChevronLeft
} from "lucide-react";

const CATEGORIES = ["Music", "Food", "Technology", "Art", "Sports", "Health"];
const CITIES = ["Beijing", "Shanghai", "Chengdu", "Shenzhen", "Hangzhou"];

export function CreateEvent() {
    const navigate = useNavigate();
    const [loading, setLoading] = useState(false);
    const getFutureTime = (hours: number) => {
        const d = new Date();
        d.setHours(d.getHours() + hours);
        d.setMinutes(0); // Round to hour for cleaner UI
        const pad = (n: number) => n.toString().padStart(2, '0');
        return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}T${pad(d.getHours())}:${pad(d.getMinutes())}`;
    };

    const [formData, setFormData] = useState({
        title: "",
        description: "",
        city: "Beijing",
        category: "Technology",
        start_time: getFutureTime(1),
        end_time: getFutureTime(3),
        capacity: 0,
    });

    const handleChange = (e: React.ChangeEvent<HTMLInputElement | HTMLTextAreaElement | HTMLSelectElement>) => {
        const { name, value } = e.target;
        setFormData(prev => ({
            ...prev,
            [name]: name === "capacity" ? parseInt(value) || 0 : value
        }));
    };

    const handleSubmit = async (publish: boolean) => {
        if (!formData.title || !formData.description || !formData.start_time || !formData.end_time) {
            toast.error("Please fill in all required fields");
            return;
        }

        const start = new Date(formData.start_time);
        const end = new Date(formData.end_time);

        if (start >= end) {
            toast.error("End time must be after start time");
            return;
        }

        setLoading(true);
        try {
            // 1. Create the event (always creates as Draft initially)
            const res = await apiClient.post("/events", {
                ...formData,
                start_time: start.toISOString(),
                end_time: end.toISOString(),
            });

            const eventId = res.data.id;

            if (publish) {
                // 2. If user chose "Publish Now", call the publish endpoint
                await apiClient.post(`/events/${eventId}/publish`);
                toast.success("Event created and published!");
                navigate(`/events/${eventId}`);
            } else {
                toast.success("Event saved as draft!");
                navigate("/me/joins"); // Or to a "My Events" page if we had one
            }
        } catch (err: any) {
            toast.error(err.response?.data?.error?.message || "Failed to create event");
        } finally {
            setLoading(false);
        }
    };

    return (
        <main className="container mx-auto py-12 px-4 max-w-3xl">
            <button
                onClick={() => navigate(-1)}
                className="flex items-center gap-2 text-slate-500 hover:text-emerald-600 transition-colors mb-6 group"
            >
                <ChevronLeft className="w-4 h-4 transition-transform group-hover:-translate-x-1" />
                <span className="text-sm font-bold uppercase tracking-widest">Back</span>
            </button>

            <div className="mb-10">
                <h1 className="text-4xl font-extrabold tracking-tight text-slate-900 dark:text-white">
                    Publish <span className="text-emerald-600">New Event</span>
                </h1>
                <p className="text-muted-foreground mt-2">
                    Share your awesome event with the community. Start as a draft or publish immediately.
                </p>
            </div>

            <div className="glass-card p-10 rounded-3xl border-white/20 backdrop-blur-xl shadow-2xl relative overflow-hidden">
                {/* Decorative background element */}
                <div className="absolute top-0 right-0 w-64 h-64 bg-emerald-600/5 rounded-full -mr-32 -mt-32 blur-3xl pointer-events-none" />

                <div className="space-y-8 relative">
                    {/* Basic Info Section */}
                    <div className="grid grid-cols-1 gap-6">
                        <div className="space-y-2">
                            <Label className="flex items-center gap-2 text-xs font-bold uppercase tracking-widest text-muted-foreground">
                                <Type className="w-3 h-3" /> Event Title
                            </Label>
                            <Input
                                name="title"
                                placeholder="Give your event a catchy name"
                                className="bg-white/50 dark:bg-slate-900/50 border-white/30 text-lg font-bold py-6 px-4"
                                value={formData.title}
                                onChange={handleChange}
                                required
                            />
                        </div>

                        <div className="space-y-2">
                            <Label className="flex items-center gap-2 text-xs font-bold uppercase tracking-widest text-muted-foreground">
                                <AlignLeft className="w-3 h-3" /> Description
                            </Label>
                            <Textarea
                                name="description"
                                placeholder="Tell everyone what makes this event special..."
                                className="bg-white/50 dark:bg-slate-900/50 border-white/30 min-h-[150px] resize-none leading-relaxed"
                                value={formData.description}
                                onChange={handleChange}
                                required
                            />
                        </div>
                    </div>

                    <div className="h-px bg-slate-200 dark:bg-white/10" />

                    {/* Location & Category */}
                    <div className="grid grid-cols-1 sm:grid-cols-2 gap-8">
                        <div className="space-y-2">
                            <Label className="flex items-center gap-2 text-xs font-bold uppercase tracking-widest text-muted-foreground">
                                <MapPin className="w-3 h-3" /> City
                            </Label>
                            <select
                                name="city"
                                className="w-full h-12 bg-white/50 dark:bg-slate-900/50 border border-white/30 rounded-xl px-4 text-sm font-semibold focus:outline-none focus:ring-2 focus:ring-emerald-500/50 transition-all cursor-pointer"
                                value={formData.city}
                                onChange={handleChange}
                            >
                                {CITIES.map(city => (
                                    <option key={city} value={city}>{city}</option>
                                ))}
                            </select>
                        </div>

                        <div className="space-y-2">
                            <Label className="flex items-center gap-2 text-xs font-bold uppercase tracking-widest text-muted-foreground">
                                <Tag className="w-3 h-3" /> Category
                            </Label>
                            <select
                                name="category"
                                className="w-full h-12 bg-white/50 dark:bg-slate-900/50 border border-white/30 rounded-xl px-4 text-sm font-semibold focus:outline-none focus:ring-2 focus:ring-emerald-500/50 transition-all cursor-pointer"
                                value={formData.category}
                                onChange={handleChange}
                            >
                                {CATEGORIES.map(cat => (
                                    <option key={cat} value={cat}>{cat}</option>
                                ))}
                            </select>
                        </div>
                    </div>

                    {/* Times Section */}
                    <div className="grid grid-cols-1 sm:grid-cols-2 gap-8">
                        <div className="space-y-2">
                            <Label className="flex items-center gap-2 text-xs font-bold uppercase tracking-widest text-muted-foreground">
                                <Calendar className="w-3 h-3" /> Start Time
                            </Label>
                            <Input
                                name="start_time"
                                type="datetime-local"
                                className="bg-white/50 dark:bg-slate-900/50 border-white/30 h-12"
                                value={formData.start_time}
                                onChange={handleChange}
                                required
                            />
                        </div>

                        <div className="space-y-2">
                            <Label className="flex items-center gap-2 text-xs font-bold uppercase tracking-widest text-muted-foreground">
                                <Clock className="w-3 h-3" /> End Time
                            </Label>
                            <Input
                                name="end_time"
                                type="datetime-local"
                                className="bg-white/50 dark:bg-slate-900/50 border-white/30 h-12"
                                value={formData.end_time}
                                onChange={handleChange}
                                required
                            />
                        </div>
                    </div>

                    <div className="space-y-2 max-w-[200px]">
                        <Label className="flex items-center gap-2 text-xs font-bold uppercase tracking-widest text-muted-foreground">
                            <Users className="w-3 h-3" /> Capacity (Optional)
                        </Label>
                        <div className="relative">
                            <Input
                                name="capacity"
                                type="number"
                                placeholder="0 for unlimited"
                                className="bg-white/50 dark:bg-slate-900/50 border-white/30 pl-10"
                                value={formData.capacity === 0 ? "" : formData.capacity}
                                onChange={handleChange}
                            />
                            <div className="absolute left-3 top-1/2 -translate-y-1/2 text-muted-foreground/50">#</div>
                        </div>
                    </div>

                    <div className="pt-8 flex flex-col sm:flex-row gap-4">
                        <Button
                            variant="outline"
                            className="flex-1 rounded-2xl h-14 font-bold uppercase tracking-widest glass-card border-white/20 hover:bg-white/20 dark:hover:bg-slate-800 transition-all flex items-center gap-2"
                            onClick={() => handleSubmit(false)}
                            disabled={loading}
                        >
                            <Save className="w-4 h-4" /> Save as Draft
                        </Button>
                        <Button
                            className="flex-[2] rounded-2xl h-14 font-bold uppercase tracking-widest bg-emerald-600 hover:bg-emerald-700 shadow-lg shadow-emerald-500/30 flex items-center justify-center gap-2 active:scale-[0.98] transition-all"
                            onClick={() => handleSubmit(true)}
                            disabled={loading}
                        >
                            <Send className="w-4 h-4" /> {loading ? "Publishing..." : "Publish Event Now"}
                        </Button>
                    </div>
                </div>
            </div>
        </main>
    );
}
