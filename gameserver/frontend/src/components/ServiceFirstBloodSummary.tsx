import { Box, Space, Text, Tooltip } from "@mantine/core";
import { FaCrosshairs, FaDroplet, FaShieldHalved } from "react-icons/fa6";
import { ServiceRoundSummary, ServiceStatusInfo } from "../scripts/query";

export const ServiceFirstBloodSummary = ({
    serviceInfo,
    summary,
}: {
    serviceInfo?: ServiceStatusInfo;
    summary?: ServiceRoundSummary;
}) => {
    const hasFirstBlood = serviceInfo?.first_blood_team_name != null;

    return (
        <Box className="center-flex-col" style={{ width: "100%" }}>
            <Tooltip
                label={
                    hasFirstBlood
                        ? `First blood: ${serviceInfo?.first_blood_team_name} at round ${serviceInfo?.first_blood_round}`
                        : "No first blood yet"
                }
                position="top"
                withArrow
            >
                <Box display="flex" style={{ alignItems: "center" }}>
                    <FaDroplet
                        size={14}
                        style={{
                            color: hasFirstBlood
                                ? "var(--mantine-color-red-6)"
                                : "var(--mantine-color-gray-6)",
                        }}
                    />
                    <Space w="xs" />
                    <Text size="sm" truncate="end" maw={120}>
                        {hasFirstBlood
                            ? `${serviceInfo?.first_blood_team_name} (R${serviceInfo?.first_blood_round})`
                            : "—"}
                    </Text>
                </Box>
            </Tooltip>
            <Space h="4px" />
            <Box
                display="flex"
                style={{ alignItems: "center", justifyContent: "center", gap: 10 }}
            >
                <Tooltip label="Exploiters last round" position="top" withArrow>
                    <Box className="center-flex">
                        <FaCrosshairs size={13} />
                        <Space w={4} />
                        <Text size="sm">{summary?.exploiters ?? 0}</Text>
                    </Box>
                </Tooltip>
                <Tooltip label="Victims last round" position="top" withArrow>
                    <Box className="center-flex">
                        <FaShieldHalved size={13} />
                        <Space w={4} />
                        <Text size="sm">{summary?.victims ?? 0}</Text>
                    </Box>
                </Tooltip>
            </Box>
        </Box>
    );
};
