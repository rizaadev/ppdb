import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useState,
  type ReactNode,
} from 'react';
import {
  apiRequest,
  readStoredSession,
  writeStoredSession,
} from '@/core/api/client';
import type {
  AuthTokenBundle,
  CurrentUser,
  RequestOTPResult,
  TenantRole,
} from '@/core/types/domain';

export type OtpPurpose = 'register' | 'login' | 'reset_pin';

interface AuthContextValue {
  user: CurrentUser | null;
  loading: boolean;
  tenantSlug: string;
  isStaff: boolean;
  hasAnyRole: (...roles: TenantRole[]) => boolean;
  hasUnitAccess: (unitId: string, ...roles: TenantRole[]) => boolean;
  requestOtp: (whatsappPhone: string, purpose: OtpPurpose) => Promise<RequestOTPResult>;
  verifyOtp: (params: {
    whatsappPhone: string;
    purpose: 'register' | 'login';
    code: string;
    fullName?: string;
    password?: string;
  }) => Promise<CurrentUser>;
  resetPasswordWithOtp: (params: {
    whatsappPhone: string;
    code: string;
    newPassword: string;
  }) => Promise<CurrentUser>;
  loginWithPassword: (identifier: string, password: string) => Promise<CurrentUser>;
  logout: () => Promise<void>;
  refreshUser: () => Promise<void>;
}

const AuthContext = createContext<AuthContextValue | null>(null);

export function AuthProvider({
  tenantSlug,
  children,
}: {
  tenantSlug: string;
  children: ReactNode;
}) {
  const [user, setUser] = useState<CurrentUser | null>(null);
  const [loading, setLoading] = useState(true);

  const loadMe = useCallback(async () => {
    const stored = readStoredSession(tenantSlug);
    if (!stored) {
      setUser(null);
      setLoading(false);
      return;
    }

    setLoading(true);
    try {
      const me = await apiRequest<CurrentUser>('/api/v1/me', {
        tenantSlug,
        onSessionExpired: () => setUser(null),
      });
      setUser(me);
    } catch {
      writeStoredSession(tenantSlug, null);
      setUser(null);
    } finally {
      setLoading(false);
    }
  }, [tenantSlug]);

  useEffect(() => {
    void loadMe();
  }, [loadMe]);

  const applyTokenBundle = useCallback(
    (bundle: AuthTokenBundle): CurrentUser => {
      writeStoredSession(tenantSlug, {
        accessToken: bundle.access_token,
        refreshToken: bundle.refresh_token,
      });
      setUser(bundle.user);
      return bundle.user;
    },
    [tenantSlug],
  );

  const requestOtp = useCallback(
    async (whatsappPhone: string, purpose: OtpPurpose) => {
      return apiRequest<RequestOTPResult>('/api/v1/auth/otp/request', {
        method: 'POST',
        tenantSlug,
        skipAuth: true,
        body: { whatsapp_phone: whatsappPhone, purpose },
      });
    },
    [tenantSlug],
  );

  const verifyOtp = useCallback(
    async (params: {
      whatsappPhone: string;
      purpose: 'register' | 'login';
      code: string;
      fullName?: string;
      password?: string;
    }) => {
      const bundle = await apiRequest<AuthTokenBundle>('/api/v1/auth/otp/verify', {
        method: 'POST',
        tenantSlug,
        skipAuth: true,
        body: {
          whatsapp_phone: params.whatsappPhone,
          purpose: params.purpose,
          code: params.code,
          full_name: params.fullName,
          password: params.password,
        },
      });
      return applyTokenBundle(bundle);
    },
    [applyTokenBundle, tenantSlug],
  );

  const resetPasswordWithOtp = useCallback(
    async (params: { whatsappPhone: string; code: string; newPassword: string }) => {
      const bundle = await apiRequest<AuthTokenBundle>('/api/v1/auth/password/reset', {
        method: 'POST',
        tenantSlug,
        skipAuth: true,
        body: {
          whatsapp_phone: params.whatsappPhone,
          code: params.code,
          new_password: params.newPassword,
        },
      });
      return applyTokenBundle(bundle);
    },
    [applyTokenBundle, tenantSlug],
  );

  const loginWithPassword = useCallback(
    async (identifier: string, password: string) => {
      const bundle = await apiRequest<AuthTokenBundle>('/api/v1/auth/login', {
        method: 'POST',
        tenantSlug,
        skipAuth: true,
        body: { identifier, password },
      });
      return applyTokenBundle(bundle);
    },
    [applyTokenBundle, tenantSlug],
  );

  const logout = useCallback(async () => {
    const stored = readStoredSession(tenantSlug);
    if (stored?.refreshToken) {
      try {
        await apiRequest('/api/v1/auth/logout', {
          method: 'POST',
          tenantSlug,
          body: { refresh_token: stored.refreshToken },
        });
      } catch {
        // Always clear local session on sign-out
      }
    }
    writeStoredSession(tenantSlug, null);
    setUser(null);
  }, [tenantSlug]);

  const hasAnyRole = useCallback(
    (...roles: TenantRole[]) => {
      if (!user) return false;
      if (user.is_platform_admin) return true;
      return user.roles.some((scope) => roles.includes(scope.role));
    },
    [user],
  );

  const hasUnitAccess = useCallback(
    (unitId: string, ...roles: TenantRole[]) => {
      if (!user) return false;
      if (user.is_platform_admin) return true;
      return user.roles.some((scope) => {
        const unitMatches = !scope.unit_id || !unitId || scope.unit_id === unitId;
        if (!unitMatches) return false;
        return roles.length === 0 || roles.includes(scope.role);
      });
    },
    [user],
  );

  const isStaff = Boolean(user && (user.is_platform_admin || user.roles.length > 0));

  return (
    <AuthContext.Provider
      value={{
        user,
        loading,
        tenantSlug,
        isStaff,
        hasAnyRole,
        hasUnitAccess,
        requestOtp,
        verifyOtp,
        resetPasswordWithOtp,
        loginWithPassword,
        logout,
        refreshUser: loadMe,
      }}
    >
      {children}
    </AuthContext.Provider>
  );
}

export function useAuth(): AuthContextValue {
  const ctx = useContext(AuthContext);
  if (!ctx) {
    throw new Error('useAuth must be used inside AuthProvider');
  }
  return ctx;
}
