import { useEffect, useState } from 'react';
import { useSearchParams, useNavigate } from 'react-router-dom';
import { toast } from 'sonner';

export function VerifyEmail() {
    const [searchParams] = useSearchParams();
    const navigate = useNavigate();
    const [status, setStatus] = useState<'verifying' | 'success' | 'error'>('verifying');

    const token = searchParams.get('token');

    useEffect(() => {
        if (!token) {
            setStatus('error');
            toast.error('Invalid verification link.');
            return;
        }

        const verify = async () => {
            try {
                const res = await fetch('/api/auth/verify-email/confirm', {
                    method: 'POST',
                    headers: {
                        'Content-Type': 'application/json',
                    },
                    body: JSON.stringify({ token }),
                });

                if (!res.ok) {
                    throw new Error('Verification failed');
                }

                setStatus('success');
                toast.success('Email verified successfully!');

                // Redirect to login after a short delay
                setTimeout(() => {
                    navigate('/login');
                }, 2000);

            } catch (error) {
                console.error('Verification error:', error);
                setStatus('error');
                toast.error('Failed to verify email. The link may be invalid or expired.');
            }
        };

        verify();
    }, [token, navigate]);

    return (
        <div className="min-h-screen flex items-center justify-center bg-gray-50 dark:bg-gray-900 px-4">
            <div className="max-w-md w-full space-y-8 p-8 bg-white dark:bg-gray-800 rounded-lg shadow-lg text-center">
                {status === 'verifying' && (
                    <>
                        <h2 className="text-2xl font-bold mb-4">Verifying your email...</h2>
                        <div className="animate-spin rounded-full h-12 w-12 border-b-2 border-primary mx-auto"></div>
                    </>
                )}

                {status === 'success' && (
                    <>
                        <h2 className="text-2xl font-bold text-green-600 mb-2">Verified!</h2>
                        <p className="text-gray-600 dark:text-gray-300">
                            Your email has been successfully verified. Redirecting to login...
                        </p>
                    </>
                )}

                {status === 'error' && (
                    <>
                        <h2 className="text-2xl font-bold text-red-600 mb-2">Verification Failed</h2>
                        <p className="text-gray-600 dark:text-gray-300 mb-6">
                            The verification link is invalid or has expired.
                        </p>
                        <button
                            onClick={() => navigate('/login')}
                            className="px-4 py-2 bg-primary text-white rounded hover:bg-primary/90"
                        >
                            Go to Login
                        </button>
                    </>
                )}
            </div>
        </div>
    );
}
